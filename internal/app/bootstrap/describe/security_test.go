package describe

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

// stubSessionStore stands in for a configured session store. The agreement spec
// only asks the verifier what it can check, never to check anything.
type stubSessionStore struct{ sessionkit.Store }

// schemeNames lists the descriptors by name, which is the form the other two
// derivations answer in.
func schemeNames(schemes []apidoc.SecurityScheme) []string {
	names := make([]string, 0, len(schemes))
	for _, s := range schemes {
		names = append(names, s.Name)
	}
	return names
}

// everyCredential turns on every credential kind a configuration can enable,
// with everything the verifier needs to build one of each. The agreement spec
// rests on it: a scheme only some of the derivations know about shows up there
// as a disagreement.
func everyCredential() *appconfig.Auth {
	return &appconfig.Auth{
		Mode:    appconfig.AuthModeHS256,
		Secret:  "local-development-secret",
		APIKeys: []string{"local-development-key"},
		Session: configuredSession(),
	}
}

// configuredSession is the session settings a deployment resolves to.
func configuredSession() *appconfig.AuthSession {
	return &appconfig.AuthSession{
		Store:                 appconfig.AuthSessionStoreRedis,
		CookieName:            appconfig.DefaultSessionCookieName,
		CookieSameSite:        appconfig.AuthSessionSameSiteLax,
		AbsoluteTTL:           appconfig.DefaultSessionAbsoluteTTL,
		PersistentAbsoluteTTL: appconfig.DefaultSessionPersistentAbsoluteTTL,
		IdleTTL:               appconfig.DefaultSessionIdleTTL,
	}
}

var _ = Describe("securitySchemes", func() {
	It("publishes nothing when nothing is configured", func() {
		Expect(securitySchemes(&appconfig.Auth{})).To(BeNil())
		Expect(securitySchemes(nil)).To(BeNil())
	})

	It("publishes the JWT scheme as a bearer token", func() {
		schemes := securitySchemes(&appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: "s"})

		Expect(schemes).To(HaveLen(1))
		Expect(schemes[0].Name).To(Equal(authkit.SchemeJWT))
		Expect(schemes[0].Type).To(Equal(apidoc.SecuritySchemeTypeHTTP))
		Expect(schemes[0].Scheme).To(Equal(apidoc.SecuritySchemeBearer))
	})

	It("publishes the API key scheme with the header it is read from", func() {
		schemes := securitySchemes(&appconfig.Auth{
			APIKeys: []string{"k"}, APIKeyHeader: "X-Tenant-Key",
		})

		Expect(schemes).To(HaveLen(1))
		Expect(schemes[0].Name).To(Equal(authkit.SchemeAPIKey))
		Expect(schemes[0].In).To(Equal(apidoc.SecuritySchemeInHeader))
		Expect(schemes[0].HeaderName).To(Equal("X-Tenant-Key"))
	})

	It("falls back to the header the verifier falls back to", func() {
		schemes := securitySchemes(&appconfig.Auth{APIKeys: []string{"k"}})

		Expect(schemes[0].HeaderName).To(Equal(appconfig.DefaultAPIKeyHeader))
	})

	// A caller can still present a key when the application verifies it
	// somewhere else, so a document omitting the scheme would be wrong.
	It("publishes the API key scheme when keys are verified elsewhere", func() {
		schemes := securitySchemes(&appconfig.Auth{APIKeysExternal: true})

		Expect(schemeNames(schemes)).To(Equal([]string{authkit.SchemeAPIKey}))
	})

	// A cookie is the only credential a browser can hold safely, and the document
	// has to say which cookie and how to get one.
	It("publishes the session scheme as the cookie it is carried in", func() {
		schemes := securitySchemes(&appconfig.Auth{Session: configuredSession()})

		Expect(schemes).To(HaveLen(1))
		Expect(schemes[0].Name).To(Equal(authkit.SchemeSession))
		Expect(schemes[0].Type).To(Equal(apidoc.SecuritySchemeTypeAPIKey))
		Expect(schemes[0].In).To(Equal(apidoc.SecuritySchemeInCookie))
		Expect(schemes[0].HeaderName).To(Equal(appconfig.DefaultSessionCookieName))
		Expect(schemes[0].Description).NotTo(BeEmpty())
	})

	// The document names whatever this deployment configured, which is why it is
	// generated with the settings of the environment it describes.
	It("publishes the cookie name this deployment configured", func() {
		session := configuredSession()
		session.CookieName = "app_session"

		schemes := securitySchemes(&appconfig.Auth{Session: session})

		Expect(schemes[0].HeaderName).To(Equal("app_session"))
	})

	Describe("agreement with the other derivations of the same question", func() {
		// Three places answer "which credentials does this application take":
		// authkit.ConfiguredSchemes (configuration only), this file (the
		// document), and Verifier.Schemes (what was actually built). They used
		// to be three independent copies of the same conditions. This is the net
		// under adding a fourth credential kind and remembering only two of them.
		It("names the same schemes as ConfiguredSchemes and as the verifier", func() {
			cfg := everyCredential()
			verifier, err := authkit.NewVerifier(cfg, nil, stubSessionStore{})
			Expect(err).NotTo(HaveOccurred())

			configured := authkit.ConfiguredSchemes(cfg)
			Expect(configured).NotTo(BeEmpty())
			Expect(schemeNames(securitySchemes(cfg))).To(Equal(configured))
			Expect(verifier.Schemes()).To(Equal(configured))
		})

		// The one place the three are meant to disagree. AUTH_API_KEYS_EXTERNAL
		// says keys are accepted and verified by code this configuration cannot
		// see; until that code hands the verifier a store, the verifier has
		// nothing to check them with. Pinning the difference keeps someone from
		// "fixing" it by making the document follow the verifier, which would
		// stop documenting a credential callers really can present.
		It("still documents a key scheme the verifier cannot check yet", func() {
			cfg := &appconfig.Auth{APIKeysExternal: true}
			verifier, err := authkit.NewVerifier(cfg, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(schemeNames(securitySchemes(cfg))).To(Equal([]string{authkit.SchemeAPIKey}))
			Expect(authkit.ConfiguredSchemes(cfg)).To(Equal([]string{authkit.SchemeAPIKey}))
			Expect(verifier.Schemes()).To(BeEmpty())
		})
	})

	// Dropping it would publish a document silent about a way into the
	// application; inventing one would tell callers to do the wrong thing.
	Describe("a scheme with no descriptor", func() {
		It("stops rather than publishing a document that leaves it out", func() {
			Expect(func() { describeScheme("carrier-pigeon", &appconfig.Auth{}) }).
				To(PanicWith(ContainSubstring("carrier-pigeon")))
		})
	})
})
