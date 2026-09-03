package keystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	enclikeystore "github.com/ensoria/encli/pkg/keystore"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// dbStore answers key lookups from a SQL database.
//
// ⚠ Withdrawing a key has to take effect everywhere at once, so nothing here
// caches a row. The lookup is a primary-key read of one narrow row, which is
// the cheapest thing a database does; a cache in front of it would buy very
// little and would keep a deleted key working on whichever node still held it.
type dbStore struct {
	db    *sql.DB
	query string
	now   func() time.Time
}

// DBStore is a key store backed by a database: everything a verifier needs,
// plus the readiness check the startup sequence runs.
//
// The check is on this type rather than on authkit.KeyStore because only a
// database has storage that can be absent. Returning it here is also what stops
// the check from being forgotten — whoever builds a database store is handed
// something that has it.
type DBStore interface {
	authkit.KeyStore
	Ready(ctx context.Context) error
}

// DBOption adjusts a database-backed store. There is one, and it exists so that
// a test can decide what "now" is when judging an expiry.
type DBOption func(*dbStore)

// WithDBClock replaces the clock the store compares expiry against. Passing nil
// leaves it alone.
func WithDBClock(now func() time.Time) DBOption {
	return func(s *dbStore) {
		if now != nil {
			s.now = now
		}
	}
}

// NewDB reads keys from the table `encli auth keystore init` creates.
//
// The statement comes from the shared format package, built once here rather
// than at every lookup. Taking it from there rather than writing it out is what
// makes a renamed column reach this query too, instead of leaving an
// application that starts cleanly and fails every lookup.
func NewDB(db *sql.DB, driver string, opts ...DBOption) (DBStore, error) {
	if db == nil {
		return nil, errors.New("keystore: no database to read keys from")
	}
	query, err := enclikeystore.SelectByFingerprintSQL(driver)
	if err != nil {
		return nil, err
	}

	store := &dbStore{db: db, query: query, now: time.Now}
	for _, opt := range opts {
		opt(store)
	}
	return store, nil
}

// readinessFingerprint is the value the probe looks up. It is not a fingerprint
// — Fingerprint only ever produces 64 hex characters — so it can never collide
// with a real key, and a row coming back would be someone else's doing.
const readinessFingerprint = "readiness-probe"

// Ready reports whether the table this store reads is actually there, by
// running the lookup itself against a fingerprint no key can have.
//
// A missing table, a renamed column, the wrong database, a role without SELECT:
// all of them come back here rather than on the first request that presents an
// API key. That timing is the whole point. Without it the application starts
// cleanly and then answers 503 to a caller, in production, possibly days after
// the deploy — and the most likely cause is not exotic, it is that
// `encli auth keystore init` was never run on this environment.
//
// It uses the same statement a lookup uses, so it checks the real contract
// rather than an approximation of it, and needs no dialect-specific
// introspection query. No row comes back, which is the expected answer.
func (s *dbStore) Ready(ctx context.Context) error {
	if _, err := s.read(ctx, readinessFingerprint); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("keystore: the %s table cannot be read: %w. "+
			"Create it with `encli auth keystore init` for this environment, "+
			"or point AUTH_KEYSTORE_DB_* at the database that has it", enclikeystore.TableName, err)
	}
	return nil
}

// Lookup returns the caller a key belongs to.
//
// The key never reaches the database or an error message: only its fingerprint
// does. An unknown or expired key is reported as authkit.ErrKeyNotFound, and
// everything else as an error meaning the store could not be asked — the
// difference between one caller being told no and every caller being told no
// during an outage.
func (s *dbStore) Lookup(ctx context.Context, key string) (*authkit.Principal, error) {
	if key == "" {
		return nil, authkit.ErrKeyNotFound
	}

	fingerprint := enclikeystore.Fingerprint(key)
	row, err := s.read(ctx, fingerprint)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, authkit.ErrKeyNotFound
	case err != nil:
		return nil, fmt.Errorf("keystore: reading the key record %s: %w", short(fingerprint), err)
	}

	// An expired key is a definite answer rather than a failure: the store was
	// asked and said no. It is kept apart from "no such key" only in the error
	// text, because the two mean the same thing to the caller and very
	// different things to whoever has to explain the refusal.
	if row.expiresAt.Valid && !row.expiresAt.Time.After(s.now().UTC()) {
		return nil, fmt.Errorf("%w: the key expired at %s",
			authkit.ErrKeyNotFound, row.expiresAt.Time.UTC().Format(time.RFC3339))
	}
	if err := validateSubject(row.subject, fingerprint); err != nil {
		return nil, err
	}

	return &authkit.Principal{
		Subject: row.subject,
		Scopes:  enclikeystore.DecodeScopes(row.scopes),
		Scheme:  authkit.SchemeAPIKey,
	}, nil
}

// keyRow is one record as the table holds it.
//
// ⚠ The scan order is the column order of the shared statement — subject,
// scopes, expires_at. Reordering it here without reordering it there mixes the
// values up silently, since two of the three are strings.
type keyRow struct {
	subject   string
	scopes    string
	expiresAt sql.NullTime
}

// read runs the lookup for one fingerprint. sql.ErrNoRows reaches the caller
// unwrapped, because the two callers read it as different things: a key that
// does not exist, and a table that does.
func (s *dbStore) read(ctx context.Context, fingerprint string) (*keyRow, error) {
	var row keyRow
	err := s.db.QueryRowContext(ctx, s.query, fingerprint).
		Scan(&row.subject, &row.scopes, &row.expiresAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}
