package sessionkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	enscache "github.com/ensoria/cache/pkg/cache"
)

// The two kinds of key the store keeps, namespaced within whatever prefix the
// cache was built with.
const (
	recordKeyPrefix  = "session:"
	revokedKeyPrefix = "revoked:"
)

// cacheStore keeps sessions in a cache.Cache.
//
// One implementation serves both backends. cacheredis and cachememory differ in
// where the bytes go, not in what the operations mean, so a second
// implementation would only be a second place for the rules below to drift.
// That also makes the memory store worth testing against: it is the same code,
// so a test that passes on it is not exercising a simplified imitation.
type cacheStore struct {
	cache enscache.Cache
	cfg   *Config
	now   func() time.Time
}

// Option adjusts a store. There is one, and it exists for tests: deadlines are
// most of what this package does, and a test that has to wait for them is a
// test nobody runs.
type Option func(*cacheStore)

// WithClock replaces the clock the store reads. Passing nil leaves it alone.
func WithClock(now func() time.Time) Option {
	return func(s *cacheStore) {
		if now != nil {
			s.now = now
		}
	}
}

// NewStore keeps sessions in the given cache.
//
// ⚠ The cache must be a single shared store — cacheredis in a deployment, or
// cachememory when there is deliberately nothing to share (tests, or running
// without a Redis).
//
// ⚠ Never hand it cachetiered or cacheotter. Those keep a copy in the process,
// and a session read from a process-local copy outlives its own revocation on
// the node holding it: signing out returns, the next request goes to that node,
// and the caller is still signed in. Being able to take a session back is the
// whole reason it is kept on the server, so the layer that would make lookups
// cheaper is exactly the layer that cannot be used.
func NewStore(cache enscache.Cache, cfg *Config, opts ...Option) (Store, error) {
	if cache == nil {
		return nil, errors.New("sessionkit: no cache to keep sessions in")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	store := &cacheStore{cache: cache, cfg: cfg, now: time.Now}
	for _, opt := range opts {
		opt(store)
	}
	return store, nil
}

func (s *cacheStore) Create(ctx context.Context, snapshot *Snapshot, persistent bool) (*Session, error) {
	if snapshot == nil || snapshot.Subject == "" {
		return nil, errors.New("sessionkit: a session needs a subject to belong to")
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}

	now := s.now()
	session := &Session{
		ID:         id,
		Snapshot:   snapshot.Clone(),
		Persistent: persistent,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.cfg.AbsoluteTTLFor(persistent)),
		LastSeenAt: now,
	}
	if err := s.write(ctx, session, now); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *cacheStore) Lookup(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, ErrSessionNotFound
	}

	session, err := enscache.Get[*Session](ctx, s.cache, recordKeyPrefix+id)
	if err != nil {
		if errors.Is(err, enscache.ErrCacheMiss) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("sessionkit: reading session %s: %w", redacted(id), err)
	}
	if session == nil || session.Snapshot == nil {
		// A record that decoded into nothing usable is not a session. Reporting
		// it as gone is the honest answer, and it lets the caller clear the
		// cookie pointing at it.
		return nil, ErrSessionNotFound
	}

	now := s.now()
	if !now.Before(session.ExpiresAt) || !now.Before(s.idleDeadline(session)) {
		// The store's own expiry should have removed it already. Reaching here
		// means a clock moved or an entry outlived its ttl, and the deadline in
		// the record is the one that was promised.
		return nil, s.forget(ctx, id)
	}

	revoked, err := s.revokedAt(ctx, session.Snapshot.Subject)
	if err != nil {
		return nil, err
	}
	if !revoked.IsZero() && !session.CreatedAt.After(revoked) {
		return nil, s.forget(ctx, id)
	}

	if err := s.touch(ctx, session, now); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *cacheStore) Revoke(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if err := s.cache.Delete(ctx, recordKeyPrefix+id); err != nil {
		return fmt.Errorf("sessionkit: ending session %s: %w", redacted(id), err)
	}
	return nil
}

// RevokeSubject records the moment every session the subject holds stopped
// being valid, rather than hunting down the sessions themselves.
//
// There is no index from a subject to its sessions, and building one over a
// key-value store means a list that two concurrent sign-ins race to rewrite and
// that nothing ever prunes. A single timestamp has none of that: one write, no
// coordination, and Lookup refuses any session created before it.
//
// The cost is one extra read per lookup, which is the trade this makes
// deliberately. A project where that read matters more than the simplicity can
// implement Store over its storage engine's own set operations — which is what
// Store being an interface is for.
//
// The marker is kept for as long as the longest-lived session could be, and no
// longer: after that, no session created before the revocation can still exist,
// so there is nothing left for it to refuse.
func (s *cacheStore) RevokeSubject(ctx context.Context, subject string) error {
	if subject == "" {
		return errors.New("sessionkit: no subject to revoke sessions for")
	}
	if err := s.cache.Set(ctx, revokedKeyPrefix+subject, s.now(), s.cfg.maxAbsoluteTTL()); err != nil {
		return fmt.Errorf("sessionkit: revoking the sessions of %q: %w", subject, err)
	}
	return nil
}

// revokedAt returns when the subject's sessions were last revoked wholesale,
// or the zero time when they never were.
func (s *cacheStore) revokedAt(ctx context.Context, subject string) (time.Time, error) {
	revoked, err := enscache.Get[time.Time](ctx, s.cache, revokedKeyPrefix+subject)
	if err != nil {
		if errors.Is(err, enscache.ErrCacheMiss) {
			return time.Time{}, nil
		}
		// Not knowing whether a revocation happened is not the same as knowing
		// it did not. Answering "no" here would keep serving sessions that an
		// administrator has already ended.
		return time.Time{}, fmt.Errorf("sessionkit: reading the revocation marker for %q: %w", subject, err)
	}
	return revoked, nil
}

// touch moves the idle deadline forward, skipping the write while the record is
// still recent enough for the deadline it already carries (see
// idleRefreshDivisor).
func (s *cacheStore) touch(ctx context.Context, session *Session, now time.Time) error {
	if now.Sub(session.LastSeenAt) < s.cfg.idleRefreshInterval() {
		return nil
	}
	session.LastSeenAt = now
	return s.write(ctx, session, now)
}

// write stores the session under the shorter of its two deadlines, so that the
// store expires it on its own even if nothing ever looks it up again.
func (s *cacheStore) write(ctx context.Context, session *Session, now time.Time) error {
	ttl := s.remaining(session, now)
	if ttl <= 0 {
		// Handing a non-positive ttl to the cache deletes the key instead of
		// storing it, which would quietly turn a Create into a no-op.
		return fmt.Errorf("sessionkit: the session for %q is already past its deadline",
			session.Snapshot.Subject)
	}
	if err := s.cache.Set(ctx, recordKeyPrefix+session.ID, session, ttl); err != nil {
		return fmt.Errorf("sessionkit: storing session %s: %w", redacted(session.ID), err)
	}
	return nil
}

// remaining is how long the session has left: the nearer of its two deadlines.
func (s *cacheStore) remaining(session *Session, now time.Time) time.Duration {
	return min(session.ExpiresAt.Sub(now), s.idleDeadline(session).Sub(now))
}

// idleDeadline is when the session lapses if nothing uses it.
func (s *cacheStore) idleDeadline(session *Session) time.Time {
	return session.LastSeenAt.Add(s.cfg.IdleTTL)
}

// forget removes a record that turned out to be unusable and reports the
// session as gone.
//
// The deletion is best-effort: the answer is already known, and failing to
// clean up does not change it — the record's own deadline will collect it.
func (s *cacheStore) forget(ctx context.Context, id string) error {
	_ = s.cache.Delete(ctx, recordKeyPrefix+id)
	return ErrSessionNotFound
}

// redactedIDPrefix is how much of a session id an error message may carry.
// Enough to tell two apart in a log, far too little to use as one.
const redactedIDPrefix = 8

// redacted shortens a session id for an error message. The id is the credential
// itself, so a message repeating it in full puts a working session in the logs.
func redacted(id string) string {
	if len(id) <= redactedIDPrefix {
		return "…"
	}
	return id[:redactedIDPrefix] + "…"
}
