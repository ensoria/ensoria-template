package sessionkit_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

// The deadlines the specs work in. They are far apart so that a spec advancing
// the clock past one is unambiguous about which one it crossed.
const (
	testAbsoluteTTL           = 2 * time.Hour
	testPersistentAbsoluteTTL = 240 * time.Hour
	testIdleTTL               = 30 * time.Minute
	// testIdleRefresh mirrors the store's own refresh interval (IdleTTL/10).
	// The specs need it to sit on both sides of that threshold.
	testIdleRefresh = testIdleTTL / 10
)

// errStoreDown stands in for the storage engine being unreachable.
var errStoreDown = errors.New("connection refused")

// testConfig is the configuration the store specs run under.
func testConfig() *sessionkit.Config {
	return &sessionkit.Config{
		CookieName:            "__Host-session",
		CookieSameSite:        http.SameSiteLaxMode,
		CookieSecure:          true,
		AbsoluteTTL:           testAbsoluteTTL,
		PersistentAbsoluteTTL: testPersistentAbsoluteTTL,
		IdleTTL:               testIdleTTL,
	}
}

// clock is a hand-wound clock, so that a spec about a deadline days away runs
// in microseconds.
type clock struct{ at time.Time }

func newClock() *clock {
	// A fixed instant rather than time.Now, so a failure reads the same on
	// every run.
	return &clock{at: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) now() time.Time          { return c.at }
func (c *clock) advance(d time.Duration) { c.at = c.at.Add(d) }

// failingCache is a working cache that can be told to stop answering, so that
// the specs can tell "there is no such session" apart from "the store could not
// be asked" — the distinction the whole package is built around.
//
// failOn decides which keys are affected: an outage that swallows the whole
// store and one that only hides the revocation marker lead to different bugs,
// and only one of them is caught by failing everything.
type failingCache struct {
	enscache.Cache
	failOn func(key string) bool
}

func newFailingCache(inner enscache.Cache, failOn func(key string) bool) *failingCache {
	return &failingCache{Cache: inner, failOn: failOn}
}

// anyKey fails every operation.
func anyKey(string) bool { return true }

// revocationKeys fails only the lookup of a subject's revocation marker.
func revocationKeys(key string) bool { return strings.HasPrefix(key, "revoked:") }

// sessionKeys fails only the lookup of a session record.
func sessionKeys(key string) bool { return strings.HasPrefix(key, "session:") }

func (c *failingCache) Get(ctx context.Context, key string) (any, error) {
	if c.failOn(key) {
		return nil, errStoreDown
	}
	return c.Cache.Get(ctx, key)
}

func (c *failingCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if c.failOn(key) {
		return errStoreDown
	}
	return c.Cache.Set(ctx, key, value, ttl)
}

func (c *failingCache) Delete(ctx context.Context, key string) error {
	if c.failOn(key) {
		return errStoreDown
	}
	return c.Cache.Delete(ctx, key)
}

// newMemoryCache is the shared store the specs run against. It is the same
// implementation a deployment uses with Redis underneath it, so what passes
// here is not passing against a simplified imitation.
func newMemoryCache() enscache.Cache {
	return cachememory.New("test")
}

// snapshotOf builds a caller to create sessions for.
func snapshotOf(subject string) *sessionkit.Snapshot {
	return &sessionkit.Snapshot{
		Subject: subject,
		Scopes:  []string{"orders:read"},
		Claims:  map[string]any{"org": "acme"},
	}
}
