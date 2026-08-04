package stub_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/stub"
)

const (
	testKey    = "test-api-key"
	testEnv    = "local"
	testCaller = "payment-provider"
)

// oneKey is the smallest valid table, used wherever the table itself is not
// what a spec is about.
func oneKey() map[string]*authkit.Principal {
	return map[string]*authkit.Principal{
		testKey: {Subject: testCaller, Scopes: []string{"orders:write"}},
	}
}

var _ = Describe("APIKeyStore", func() {
	Describe("Lookup", func() {
		It("returns the caller the key belongs to", func() {
			store, err := stub.NewAPIKeyStore(testEnv, oneKey())
			Expect(err).NotTo(HaveOccurred())

			principal, err := store.Lookup(testKey)

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Subject).To(Equal(testCaller))
			Expect(principal.Scopes).To(Equal([]string{"orders:write"}))
		})

		// This is the whole reason the stub exists: the configured keys produce
		// a caller with no scopes, so any endpoint declaring one refuses them.
		It("gives the caller scopes the configured keys cannot carry", func() {
			store, err := stub.NewAPIKeyStore(testEnv, oneKey())
			Expect(err).NotTo(HaveOccurred())

			principal, _ := store.Lookup(testKey)

			Expect(principal.HasScopes([]string{"orders:write"})).To(BeTrue())
			Expect(principal.HasScopes([]string{"orders:read"})).To(BeFalse())
		})

		// The scheme is what an endpoint's Schemes declaration is matched
		// against; a caller arriving through this store came in with an API key.
		It("marks the caller as having authenticated with an API key", func() {
			store, err := stub.NewAPIKeyStore(testEnv, oneKey())
			Expect(err).NotTo(HaveOccurred())

			principal, _ := store.Lookup(testKey)

			Expect(principal.Scheme).To(Equal(authkit.SchemeAPIKey))
			Expect(principal.HasScheme([]string{authkit.SchemeAPIKey})).To(BeTrue())
		})

		// Even when the argument said otherwise: the store knows how the caller
		// got in, and the table does not get to claim it was a token.
		It("marks it as an API key even when the table says another scheme", func() {
			store, err := stub.NewAPIKeyStore(testEnv, map[string]*authkit.Principal{
				testKey: {Subject: testCaller, Scheme: authkit.SchemeJWT},
			})
			Expect(err).NotTo(HaveOccurred())

			principal, _ := store.Lookup(testKey)

			Expect(principal.Scheme).To(Equal(authkit.SchemeAPIKey))
		})

		It("refuses a key it does not hold", func() {
			store, err := stub.NewAPIKeyStore(testEnv, oneKey())
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Lookup("another-key")

			Expect(err).To(HaveOccurred())
		})

		// A rejected key is as likely to be a real one sent to the wrong place,
		// and the message ends up in a log.
		It("does not repeat the rejected key back", func() {
			store, err := stub.NewAPIKeyStore(testEnv, oneKey())
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Lookup("secret-key-of-someone-else")

			Expect(err.Error()).NotTo(ContainSubstring("secret-key-of-someone-else"))
		})

		It("tells two keys apart", func() {
			store, err := stub.NewAPIKeyStore(testEnv, map[string]*authkit.Principal{
				"key-a": {Subject: "a", Scopes: []string{"orders:write"}},
				"key-b": {Subject: "b", Scopes: []string{"orders:read"}},
			})
			Expect(err).NotTo(HaveOccurred())

			a, _ := store.Lookup("key-a")
			b, _ := store.Lookup("key-b")

			Expect(a.Subject).To(Equal("a"))
			Expect(b.Subject).To(Equal("b"))
			Expect(a.HasScopes([]string{"orders:read"})).To(BeFalse())
		})
	})

	Describe("isolation", func() {
		// The store answers what it was built with, not what the caller's map
		// says later.
		It("is unaffected by changes to the table it was built from", func() {
			keys := oneKey()
			store, err := stub.NewAPIKeyStore(testEnv, keys)
			Expect(err).NotTo(HaveOccurred())

			keys[testKey].Scopes = append(keys[testKey].Scopes, "orders:read")
			keys["added-later"] = &authkit.Principal{Subject: "intruder"}

			principal, _ := store.Lookup(testKey)
			Expect(principal.HasScopes([]string{"orders:read"})).To(BeFalse())
			_, err = store.Lookup("added-later")
			Expect(err).To(HaveOccurred())
		})

		// Application code that adds a scope to the principal it was handed must
		// not widen what the next request may do.
		It("does not let one lookup change the next", func() {
			store, err := stub.NewAPIKeyStore(testEnv, oneKey())
			Expect(err).NotTo(HaveOccurred())

			first, _ := store.Lookup(testKey)
			first.Scopes = append(first.Scopes, "orders:read")
			first.Subject = "somebody-else"

			second, _ := store.Lookup(testKey)
			Expect(second.HasScopes([]string{"orders:read"})).To(BeFalse())
			Expect(second.Subject).To(Equal(testCaller))
		})
	})

	Describe("the environments it may be built in", func() {
		DescribeTable("allows the environments a developer runs locally",
			func(environment string) {
				_, err := stub.NewAPIKeyStore(environment, oneKey())

				Expect(err).NotTo(HaveOccurred())
			},
			Entry("local", "local"),
			Entry("test", "test"),
		)

		// A stub is only safe because it cannot reach a deployment. The keys are
		// written down, so anywhere else they protect nothing.
		DescribeTable("refuses every deployed environment",
			func(environment string) {
				_, err := stub.NewAPIKeyStore(environment, oneKey())

				Expect(err).To(MatchError(ContainSubstring(environment)))
				Expect(err).To(MatchError(ContainSubstring("authkit.KeyStore")))
			},
			Entry("development", "development"),
			Entry("staging", "staging"),
			Entry("production", "production"),
		)

		// An unrecognized value must not fall through to "allowed": a typo in
		// the environment would then be the way past the check.
		It("refuses an environment it does not recognize", func() {
			_, err := stub.NewAPIKeyStore("prod", oneKey())

			Expect(err).To(HaveOccurred())
		})

		It("refuses an empty environment", func() {
			_, err := stub.NewAPIKeyStore("", oneKey())

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("what it will not be built from", func() {
		// A store that accepts nobody would look configured to the startup check
		// and refuse every request.
		It("refuses an empty table", func() {
			_, err := stub.NewAPIKeyStore(testEnv, nil)

			Expect(err).To(MatchError(ContainSubstring("no keys")))
		})

		It("refuses an empty key", func() {
			_, err := stub.NewAPIKeyStore(testEnv, map[string]*authkit.Principal{
				"": {Subject: testCaller},
			})

			Expect(err).To(MatchError(ContainSubstring("cannot be empty")))
		})

		It("refuses a key with no caller behind it", func() {
			_, err := stub.NewAPIKeyStore(testEnv, map[string]*authkit.Principal{
				testKey: nil,
			})

			Expect(err).To(MatchError(ContainSubstring("no caller")))
		})

		// Without a subject nothing identifies the caller in a log, which is the
		// one thing this store offers over the configured keys.
		It("refuses a caller with no subject", func() {
			_, err := stub.NewAPIKeyStore(testEnv, map[string]*authkit.Principal{
				testKey: {Scopes: []string{"orders:write"}},
			})

			Expect(err).To(MatchError(ContainSubstring("no subject")))
		})

		// A caller with no scopes is legitimate: it is what an endpoint
		// declaring none accepts.
		It("accepts a caller with no scopes", func() {
			store, err := stub.NewAPIKeyStore(testEnv, map[string]*authkit.Principal{
				testKey: {Subject: testCaller},
			})

			Expect(err).NotTo(HaveOccurred())
			principal, err := store.Lookup(testKey)
			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Scopes).To(BeEmpty())
		})
	})

	// The store is only useful if authkit will take it.
	It("satisfies authkit.KeyStore", func() {
		store, err := stub.NewAPIKeyStore(testEnv, oneKey())
		Expect(err).NotTo(HaveOccurred())

		var keys authkit.KeyStore = store

		Expect(keys).NotTo(BeNil())
	})
})
