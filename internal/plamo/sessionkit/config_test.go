package sessionkit_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

// configuredSession is the settings a deployment resolves to, in the shape the
// configuration package hands over.
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

var _ = Describe("Config", func() {
	Describe("NewConfig", func() {
		It("carries the settings over", func() {
			cfg, err := sessionkit.NewConfig(configuredSession())

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CookieName).To(Equal(appconfig.DefaultSessionCookieName))
			Expect(cfg.CookieSameSite).To(Equal(http.SameSiteLaxMode))
			Expect(cfg.CookieSecure).To(BeTrue())
			Expect(cfg.AbsoluteTTL).To(Equal(appconfig.DefaultSessionAbsoluteTTL))
			Expect(cfg.PersistentAbsoluteTTL).To(Equal(appconfig.DefaultSessionPersistentAbsoluteTTL))
			Expect(cfg.IdleTTL).To(Equal(appconfig.DefaultSessionIdleTTL))
		})

		It("maps the tighter SameSite", func() {
			settings := configuredSession()
			settings.CookieSameSite = appconfig.AuthSessionSameSiteStrict

			cfg, err := sessionkit.NewConfig(settings)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CookieSameSite).To(Equal(http.SameSiteStrictMode))
		})

		// Sessions being off is a state the application is allowed to be in;
		// building the machinery for them anyway is not.
		It("refuses to build sessions that were never turned on", func() {
			_, err := sessionkit.NewConfig(nil)

			Expect(err).To(MatchError(ContainSubstring("AUTH_SESSION_STORE")))
		})

		// The configuration package refuses an unknown value first. This is the
		// second line, for a Config assembled in code, where the zero SameSite
		// would otherwise write no attribute at all.
		It("refuses a SameSite it cannot write", func() {
			settings := configuredSession()
			settings.CookieSameSite = "none"

			_, err := sessionkit.NewConfig(settings)

			Expect(err).To(MatchError(ContainSubstring("SameSite")))
		})
	})

	Describe("Validate", func() {
		// This one has to be caught here because nothing else reports it: the
		// browser refuses a __Host- cookie without Secure and says nothing, so
		// every sign-in appears to succeed and then not have happened.
		It("refuses a __Host- cookie that cannot carry Secure", func() {
			settings := configuredSession()
			settings.CookieInsecure = true

			_, err := sessionkit.NewConfig(settings)

			Expect(err).To(MatchError(ContainSubstring("__Host-")))
			Expect(err).To(MatchError(ContainSubstring("AUTH_SESSION_COOKIE_INSECURE")))
			Expect(err).To(MatchError(ContainSubstring("AUTH_SESSION_COOKIE_NAME")))
		})

		// The pair that does work over plain HTTP: drop Secure and drop the
		// prefix that requires it.
		It("accepts an insecure cookie under a name without the prefix", func() {
			settings := configuredSession()
			settings.CookieInsecure = true
			settings.CookieName = "app_session"

			cfg, err := sessionkit.NewConfig(settings)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CookieSecure).To(BeFalse())
		})

		It("refuses a cookie with no name", func() {
			settings := configuredSession()
			settings.CookieName = ""

			_, err := sessionkit.NewConfig(settings)

			Expect(err).To(MatchError(ContainSubstring("AUTH_SESSION_COOKIE_NAME")))
		})

		DescribeTable("refuses a deadline that is not a deadline",
			func(apply func(*appconfig.AuthSession), key string) {
				settings := configuredSession()
				apply(settings)

				_, err := sessionkit.NewConfig(settings)

				Expect(err).To(MatchError(ContainSubstring(key)))
			},
			Entry("the absolute lifetime",
				func(s *appconfig.AuthSession) { s.AbsoluteTTL = 0 }, "AUTH_SESSION_ABSOLUTE_TTL"),
			Entry("the persistent lifetime",
				func(s *appconfig.AuthSession) { s.PersistentAbsoluteTTL = -time.Hour },
				"AUTH_SESSION_PERSISTENT_ABSOLUTE_TTL"),
			Entry("the idle limit",
				func(s *appconfig.AuthSession) { s.IdleTTL = 0 }, "AUTH_SESSION_IDLE_TTL"),
		)
	})

	Describe("AbsoluteTTLFor", func() {
		It("answers with the profile asked for", func() {
			cfg := testConfig()

			Expect(cfg.AbsoluteTTLFor(false)).To(Equal(testAbsoluteTTL))
			Expect(cfg.AbsoluteTTLFor(true)).To(Equal(testPersistentAbsoluteTTL))
		})
	})
})
