package keystore_test

import (
	"context"
	"errors"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cachememory"
	enclikeystore "github.com/ensoria/encli/pkg/keystore"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/keystore"
)

// errStoreDown stands in for the storage engine being unreachable.
var errStoreDown = errors.New("connection refused")

// recordTTL is how long the specs keep a record. Nothing here tests expiry;
// the value only has to be long enough not to interfere.
const recordTTL = time.Hour

// failingCache is a working cache that stops answering, so that the specs can
// tell "no such key" apart from "the store could not be asked".
type failingCache struct {
	enscache.Cache
}

func (c *failingCache) Get(context.Context, string) (any, error) { return nil, errStoreDown }

// store builds the key store over a cache, and returns the cache so a spec can
// put records in it the way an operator would.
func store() (authkit.KeyStore, enscache.Cache) {
	GinkgoHelper()

	cache := cachememory.New("test")
	s, err := keystore.NewRedis(cache)
	Expect(err).NotTo(HaveOccurred())
	return s, cache
}

// issue writes a record for a key, the way whatever issues keys would.
func issue(cache enscache.Cache, key string, record *enclikeystore.Record) {
	GinkgoHelper()

	Expect(cache.Set(context.Background(),
		enclikeystore.RedisKeyPrefix+enclikeystore.Fingerprint(key), record, recordTTL)).To(Succeed())
}

var _ = Describe("the Redis-backed key store", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("returns the caller a key belongs to", func() {
		keys, cache := store()
		issue(cache, "a-key", &enclikeystore.Record{
			Subject: "payment-provider", Scopes: []string{"orders:write"},
		})

		principal, err := keys.Lookup(ctx, "a-key")

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.Subject).To(Equal("payment-provider"))
		Expect(principal.Scopes).To(Equal([]string{"orders:write"}))
	})

	// Which scheme the caller used is not the record's to decide: a caller who
	// arrived with an API key authenticated with an API key.
	It("marks the caller as having used an API key", func() {
		keys, cache := store()
		issue(cache, "a-key", &enclikeystore.Record{Subject: "payment-provider"})

		principal, err := keys.Lookup(ctx, "a-key")

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.Scheme).To(Equal(authkit.SchemeAPIKey))
	})

	It("reports a key it does not know", func() {
		keys, _ := store()

		_, err := keys.Lookup(ctx, "not-a-key")

		Expect(err).To(MatchError(authkit.ErrKeyNotFound))
	})

	It("reports an empty key without asking the store", func() {
		keys, _ := store()

		_, err := keys.Lookup(ctx, "")

		Expect(err).To(MatchError(authkit.ErrKeyNotFound))
	})

	// The whole point of the fingerprint: what is stored is not usable as a key.
	It("stores the key under its fingerprint, not as itself", func() {
		keys, cache := store()
		issue(cache, "a-key", &enclikeystore.Record{Subject: "payment-provider"})

		_, err := cache.Get(ctx, enclikeystore.RedisKeyPrefix+"a-key")
		Expect(err).To(MatchError(enscache.ErrCacheMiss))

		_, err = keys.Lookup(ctx, "a-key")
		Expect(err).NotTo(HaveOccurred())
	})

	// A record nobody can be identified by is a fault in the data, not a wrong
	// key. Reporting it as unknown would send the key's owner off to check a
	// key that is perfectly correct.
	It("does not blame the caller for a record with no subject", func() {
		keys, cache := store()
		issue(cache, "a-key", &enclikeystore.Record{Scopes: []string{"orders:write"}})

		_, err := keys.Lookup(ctx, "a-key")

		Expect(err).To(HaveOccurred())
		Expect(err).NotTo(MatchError(authkit.ErrKeyNotFound))
	})

	Describe("when the store cannot be reached", func() {
		// Reporting an outage as "no such key" answers 401, which tells every
		// caller in the system that their credential is bad at the moment
		// nothing can check any of them.
		It("does not report the failure as an unknown key", func() {
			keys, err := keystore.NewRedis(&failingCache{Cache: cachememory.New("test")})
			Expect(err).NotTo(HaveOccurred())

			_, err = keys.Lookup(ctx, "a-key")

			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(MatchError(authkit.ErrKeyNotFound))
		})

		// The key is a bearer credential; an error message repeating it puts a
		// working credential in the logs.
		It("does not put the key in the error", func() {
			keys, err := keystore.NewRedis(&failingCache{Cache: cachememory.New("test")})
			Expect(err).NotTo(HaveOccurred())

			_, err = keys.Lookup(ctx, "a-secret-key")

			Expect(err.Error()).NotTo(ContainSubstring("a-secret-key"))
		})

		// Nor the whole fingerprint: enough of it to find the record is enough.
		It("names only as much of the fingerprint as identifies the record", func() {
			keys, err := keystore.NewRedis(&failingCache{Cache: cachememory.New("test")})
			Expect(err).NotTo(HaveOccurred())

			_, err = keys.Lookup(ctx, "a-secret-key")

			fingerprint := enclikeystore.Fingerprint("a-secret-key")
			Expect(err.Error()).To(ContainSubstring(fingerprint[:12]))
			Expect(err.Error()).NotTo(ContainSubstring(fingerprint))
		})
	})

	It("refuses to read keys from nowhere", func() {
		_, err := keystore.NewRedis(nil)

		Expect(err).To(HaveOccurred())
	})

	It("names itself in its errors", func() {
		keys, err := keystore.NewRedis(&failingCache{Cache: cachememory.New("test")})
		Expect(err).NotTo(HaveOccurred())

		_, err = keys.Lookup(ctx, "a-key")

		Expect(strings.HasPrefix(err.Error(), "keystore:")).To(BeTrue())
	})
})
