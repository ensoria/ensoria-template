package auth

import (
	"context"

	"github.com/ensoria/config/pkg/appconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// withSecret and withAPIKey build the configuration under test.
//
// withSecret picks hs256 because that is the only mode where a secret is read
// at all; the modes that ignore it are covered separately.
func withSecret(secret string) *appconfig.Auth {
	return &appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: secret}
}

func withAPIKey(keys ...string) *appconfig.Auth { return &appconfig.Auth{APIKeys: keys} }

var _ = Describe("the secret shipped with the template", func() {
	// The template ships a working secret so a checkout boots. Because it is
	// published, a deployment that keeps it can have its tokens forged by
	// anyone, so it must not survive past a developer's own machine.
	DescribeTable("is refused outside the environments it exists for",
		func(envVal string, allowed bool) {
			err := checkDevCredentials(envVal, withSecret(DevSecret))

			if allowed {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(envVal))
		},
		Entry("local", "local", true),
		Entry("test", "test", true),
		Entry("development", "development", false),
		Entry("staging", "staging", false),
		Entry("production", "production", false),
	)

	It("says how to fix it rather than only that it is wrong", func() {
		err := checkDevCredentials("production", withSecret(DevSecret))

		Expect(err.Error()).To(ContainSubstring("AUTH_SECRET"))
		Expect(err.Error()).To(ContainSubstring("jwks"))
	})

	It("leaves any other secret alone in every environment", func() {
		for _, envVal := range []string{"local", "test", "development", "staging", "production"} {
			Expect(checkDevCredentials(envVal, withSecret("a-secret-of-our-own"))).To(Succeed(), envVal)
		}
	})

	// An empty secret is the "nothing configured" case, which the startup check
	// in the HTTP pipeline reports; this guard must not claim it separately.
	It("says nothing about an unset secret", func() {
		Expect(checkDevCredentials("production", withSecret(""))).To(Succeed())
	})

	// Under jwks the secret verifies nothing, so a value left behind protects
	// nothing and endangers nothing. Refusing to start over it would report a
	// danger that does not exist, and tell the deployment to switch to the mode
	// it is already running.
	DescribeTable("is ignored in the modes that never read it",
		func(mode string) {
			cfg := &appconfig.Auth{
				Mode:    mode,
				Secret:  DevSecret,
				JWKSURL: "https://issuer.example.com/.well-known/jwks.json",
			}

			Expect(checkDevCredentials("production", cfg)).To(Succeed())
		},
		Entry("jwks verifies against the issuer's public keys", appconfig.AuthModeJWKS),
		Entry("no mode verifies no token at all", ""),
	)

	// The guard has to come back on its own once the secret is live again:
	// switching to hs256 is exactly the change that turns a harmless leftover
	// into the credential protecting the deployment.
	It("is refused again once the mode makes it live", func() {
		cfg := &appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: DevSecret}

		Expect(checkDevCredentials("production", cfg)).To(HaveOccurred())
	})
})

var _ = Describe("the API key shipped with the template", func() {
	It("is refused outside local and test", func() {
		err := checkDevCredentials("production", withAPIKey(DevAPIKey))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("AUTH_API_KEYS"))
	})

	// The key is refused even when it is one of several, because the published
	// key alone is enough to reach every endpoint that accepts an API key.
	It("is refused even alongside keys of our own", func() {
		err := checkDevCredentials("production", withAPIKey("a-key-of-our-own", DevAPIKey))

		Expect(err).To(HaveOccurred())
	})

	It("is allowed on a developer machine", func() {
		Expect(checkDevCredentials("local", withAPIKey(DevAPIKey))).To(Succeed())
	})

	It("leaves keys of our own alone", func() {
		Expect(checkDevCredentials("production", withAPIKey("a-key-of-our-own"))).To(Succeed())
	})

	// Unlike the secret, an API key does not belong to a mode: it is read in
	// every one of them, so the guard must not be narrowed the same way.
	DescribeTable("is refused whatever the tokens are verified with",
		func(mode string) {
			cfg := withAPIKey(DevAPIKey)
			cfg.Mode = mode

			Expect(checkDevCredentials("production", cfg)).To(HaveOccurred())
		},
		Entry("hs256", appconfig.AuthModeHS256),
		Entry("jwks", appconfig.AuthModeJWKS),
		Entry("no tokens at all", ""),
	)

	// Declaring that keys live elsewhere does not stop the configured ones from
	// being accepted: the swap happens by injecting a key store, and until that
	// is done the listed keys are still what a caller is checked against.
	It("is refused even where the keys are declared to come from elsewhere", func() {
		cfg := withAPIKey(DevAPIKey)
		cfg.APIKeysExternal = true

		Expect(checkDevCredentials("production", cfg)).To(HaveOccurred())
	})
})

var _ = Describe("the development key store", func() {
	// The configured keys carry no permissions of their own, so without this an
	// endpoint declaring any scope refuses every API key with 403.
	It("gives the configured keys the scopes the endpoints declare", func() {
		keys, err := devKeyStore("local", withAPIKey(DevAPIKey))
		Expect(err).NotTo(HaveOccurred())

		principal, err := keys.Lookup(context.Background(), DevAPIKey)

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.Subject).To(Equal(DevSubject))
		Expect(principal.HasScopes(devScopes())).To(BeTrue())
	})

	// The keys still come from the configuration, so adding one locally keeps
	// working. Only the permissions are added here.
	It("covers a key the developer added to the configuration", func() {
		keys, err := devKeyStore("local", withAPIKey(DevAPIKey, "a-key-of-my-own"))
		Expect(err).NotTo(HaveOccurred())

		principal, err := keys.Lookup(context.Background(), "a-key-of-my-own")

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.HasScopes(devScopes())).To(BeTrue())
	})

	Describe("the payment provider key", func() {
		It("holds orders:write and nothing else", func() {
			keys, err := devKeyStore("local", withAPIKey(DevAPIKey))
			Expect(err).NotTo(HaveOccurred())

			principal, err := keys.Lookup(context.Background(), DevPaymentAPIKey)

			Expect(err).NotTo(HaveOccurred())
			Expect(principal.Subject).To(Equal(DevPaymentSubject))
			Expect(principal.HasScopes([]string{"orders:write"})).To(BeTrue())
			Expect(principal.HasScopes([]string{"orders:read"})).To(BeFalse())
		})

		// It exists to make the difference between "authenticated" and
		// "permitted" observable, which it cannot do if it can do everything.
		It("cannot do what the configured key can", func() {
			keys, err := devKeyStore("local", withAPIKey(DevAPIKey))
			Expect(err).NotTo(HaveOccurred())

			payment, _ := keys.Lookup(context.Background(), DevPaymentAPIKey)
			configured, _ := keys.Lookup(context.Background(), DevAPIKey)

			Expect(payment.HasScopes(devScopes())).To(BeFalse())
			Expect(configured.HasScopes(devScopes())).To(BeTrue())
		})

		// Unlike DevAPIKey it is not in the configuration, so nothing outside
		// local and test can be holding it.
		It("is not one of the configured keys", func() {
			Expect(withAPIKey(DevAPIKey).APIKeys).NotTo(ContainElement(DevPaymentAPIKey))
		})
	})

	// A deployment gets the same key store it got before this existed, so the
	// stub cannot change what production accepts.
	DescribeTable("is not built outside the environments it exists for",
		func(envVal string) {
			keys, err := devKeyStore(envVal, withAPIKey(DevAPIKey))

			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(BeNil())
		},
		Entry("development", "development"),
		Entry("staging", "staging"),
		Entry("production", "production"),
		Entry("unrecognized", "prod"),
	)

	// An application that turned API keys off must not have them turned back on
	// here: injecting a store would make the verifier report that it accepts a
	// scheme the configuration never asked for.
	It("is not built when the configuration lists no keys", func() {
		keys, err := devKeyStore("local", withAPIKey())

		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(BeNil())
	})

	// A project setting this is expected to inject a store of its own, which
	// the stub would shadow.
	It("is not built when the keys are declared to come from elsewhere", func() {
		cfg := &appconfig.Auth{APIKeysExternal: true}

		keys, err := devKeyStore("local", cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(BeNil())
	})

	It("is not built when there is no auth configuration at all", func() {
		keys, err := devKeyStore("local", nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(BeNil())
	})

	// Handed to authkit as-is, so a caller presenting the key is authorized by
	// the same path a request takes.
	It("is what the verifier looks keys up in", func() {
		cfg := withAPIKey(DevAPIKey)
		keys, err := devKeyStore("local", cfg)
		Expect(err).NotTo(HaveOccurred())

		verifier, err := authkit.NewVerifier(cfg, keys, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(verifier.Schemes()).To(ContainElement(authkit.SchemeAPIKey))
	})
})
