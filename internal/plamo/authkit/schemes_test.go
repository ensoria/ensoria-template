package authkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

var _ = Describe("ConfiguredSchemes", func() {
	It("reports tokens when a shared secret is set", func() {
		cfg := &appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: "s"}

		Expect(authkit.ConfiguredSchemes(cfg)).To(Equal([]string{authkit.SchemeJWT}))
	})

	It("reports tokens when a key set URL is set", func() {
		cfg := &appconfig.Auth{Mode: appconfig.AuthModeJWKS, JWKSURL: "https://issuer.test/jwks"}

		Expect(authkit.ConfiguredSchemes(cfg)).To(Equal([]string{authkit.SchemeJWT}))
	})

	It("reports API keys when they are listed", func() {
		cfg := &appconfig.Auth{APIKeys: []string{"a-key"}}

		Expect(authkit.ConfiguredSchemes(cfg)).To(Equal([]string{authkit.SchemeAPIKey}))
	})

	// An application verifying keys against a database lists none of them.
	// Leaving the scheme out would drop it from the generated documentation,
	// so a caller would not know API keys are accepted at all.
	It("reports API keys when the configuration says they are verified elsewhere", func() {
		cfg := &appconfig.Auth{APIKeysExternal: true}

		Expect(authkit.ConfiguredSchemes(cfg)).To(Equal([]string{authkit.SchemeAPIKey}))
	})

	It("reports both when both are set up", func() {
		cfg := &appconfig.Auth{Secret: "s", APIKeysExternal: true}

		Expect(authkit.ConfiguredSchemes(cfg)).To(Equal([]string{authkit.SchemeJWT, authkit.SchemeAPIKey}))
	})

	It("reports nothing when nothing is set up", func() {
		Expect(authkit.ConfiguredSchemes(&appconfig.Auth{})).To(BeEmpty())
		Expect(authkit.ConfiguredSchemes(nil)).To(BeEmpty())
	})
})

var _ = Describe("what a verifier reports it can check", func() {
	It("reports tokens for a shared-secret setup", func() {
		v, err := authkit.NewVerifier(&appconfig.Auth{
			Mode: appconfig.AuthModeHS256, Secret: "s",
		}, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(v.Schemes()).To(Equal([]string{authkit.SchemeJWT}))
	})

	It("reports API keys for a key-list setup", func() {
		v, err := authkit.NewVerifier(&appconfig.Auth{APIKeys: []string{"a-key"}}, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(v.Schemes()).To(Equal([]string{authkit.SchemeAPIKey}))
	})

	// This is the case the configuration cannot see: the keys live in a store
	// the application handed over, and no key appears in the configuration.
	It("reports API keys for an injected key store with nothing in the configuration", func() {
		store := authkit.KeyStoreFunc(func(string) (*authkit.Principal, error) {
			return &authkit.Principal{Subject: "svc_1", Scheme: authkit.SchemeAPIKey}, nil
		})

		v, err := authkit.NewVerifier(&appconfig.Auth{}, store)

		Expect(err).NotTo(HaveOccurred())
		Expect(v.Schemes()).To(Equal([]string{authkit.SchemeAPIKey}))
	})

	It("reports both when both are set up", func() {
		v, err := authkit.NewVerifier(&appconfig.Auth{
			Mode: appconfig.AuthModeHS256, Secret: "s", APIKeys: []string{"a-key"},
		}, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(v.Schemes()).To(Equal([]string{authkit.SchemeJWT, authkit.SchemeAPIKey}))
	})

	It("reports nothing when nothing was set up", func() {
		v, err := authkit.NewVerifier(&appconfig.Auth{}, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(v.Schemes()).To(BeEmpty())
	})
})
