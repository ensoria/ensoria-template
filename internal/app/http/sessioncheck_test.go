package http

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// sessionParams builds a configuration with browser sessions turned on.
func sessionParams() *appconfig.Parameters {
	return &appconfig.Parameters{
		Auth: &appconfig.Auth{
			Session: &appconfig.AuthSession{
				Store:      appconfig.AuthSessionStoreRedis,
				CookieName: appconfig.DefaultSessionCookieName,
			},
		},
		CORS:      &appconfig.CORS{AllowOriginVal: "https://app.example.com"},
		AllValues: map[string]string{"AUTH_SESSION_STORE": appconfig.AuthSessionStoreRedis},
	}
}

// noSessionParams builds one with them turned off.
func noSessionParams() *appconfig.Parameters {
	return &appconfig.Parameters{
		Auth:      &appconfig.Auth{},
		CORS:      &appconfig.CORS{},
		AllValues: map[string]string{},
	}
}

// sessionModules is a module declaring an endpoint that only a session opens.
func sessionModules() []*rest.Module {
	return moduleWith(&restkit.SecuritySpec{Schemes: []string{authkit.SchemeSession}})
}

var _ = Describe("checkSessionConfiguration", func() {
	local := string(env.Local)
	production := string(env.Production)

	It("accepts an application that does not authenticate browsers at all", func() {
		Expect(checkSessionConfiguration(production, noSessionParams(), moduleWith(nil))).To(Succeed())
	})

	It("accepts a deployment that names its frontend's origin", func() {
		Expect(checkSessionConfiguration(production, sessionParams(), sessionModules())).To(Succeed())
	})

	// The same-origin deployment: the frontend is served by this application,
	// so there is no other origin to allow and CORS is not needed at all.
	It("accepts a deployment with no CORS configuration", func() {
		params := sessionParams()
		params.CORS = &appconfig.CORS{}

		Expect(checkSessionConfiguration(production, params, sessionModules())).To(Succeed())
	})

	Describe("session keys that do nothing", func() {
		// The failure this catches is silent: the value parses, it is simply
		// never read, and the deployment runs with no sessions while its author
		// believes it has them.
		It("refuses a configuration that tunes sessions without turning them on", func() {
			params := noSessionParams()
			params.AllValues["AUTH_SESSION_IDLE_TTL"] = "24h"
			params.AllValues["AUTH_SESSION_COOKIE_NAME"] = "session"

			err := checkSessionConfiguration(production, params, moduleWith(nil))

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AUTH_SESSION_COOKIE_NAME"))
			Expect(err.Error()).To(ContainSubstring("AUTH_SESSION_IDLE_TTL"))
			Expect(err.Error()).To(ContainSubstring("AUTH_SESSION_STORE"))
		})

		It("says nothing about the selector itself when it is the only key", func() {
			params := noSessionParams()
			params.AllValues["AUTH_SESSION_STORE"] = ""

			Expect(checkSessionConfiguration(production, params, moduleWith(nil))).To(Succeed())
		})
	})

	Describe("session endpoints with no store", func() {
		It("refuses endpoints that authenticate with a cookie nothing can create", func() {
			err := checkSessionConfiguration(production, noSessionParams(), sessionModules())

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AUTH_SESSION_STORE"))
			// The message has to name the other way out, or it reads as
			// "you must use sessions".
			Expect(err.Error()).To(ContainSubstring("internal/app/auth/api"))
		})
	})

	Describe("a wildcard origin", func() {
		// ⚠ The combination that gives away what the cookie is for: every site
		// becomes this application's frontend.
		It("refuses cookie authentication with CORS_ALLOW_ORIGIN=*", func() {
			params := sessionParams()
			params.CORS = &appconfig.CORS{AllowOriginVal: "*"}

			err := checkSessionConfiguration(production, params, sessionModules())

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CORS_ALLOW_ORIGIN"))
			// Both ways out have to be there: the same-origin deployment and
			// the one with a frontend somewhere else.
			Expect(err.Error()).To(ContainSubstring("unset"))
			Expect(err.Error()).To(ContainSubstring("CORS_ALLOW_CREDENTIALS"))
		})

		// Without sessions the wildcard is an ordinary open API, which is a
		// thing an application is allowed to be.
		It("leaves it alone when browsers are not authenticated with a cookie", func() {
			params := noSessionParams()
			params.CORS = &appconfig.CORS{AllowOriginVal: "*"}

			Expect(checkSessionConfiguration(production, params, moduleWith(nil))).To(Succeed())
		})
	})

	Describe("a cookie without Secure", func() {
		insecure := func() *appconfig.Parameters {
			params := sessionParams()
			params.Auth.Session.CookieInsecure = true
			params.Auth.Session.CookieName = "session"
			return params
		}

		It("allows it while developing over plain HTTP", func() {
			Expect(checkSessionConfiguration(local, insecure(), sessionModules())).To(Succeed())
		})

		It("refuses it anywhere a real user could reach", func() {
			err := checkSessionConfiguration(production, insecure(), sessionModules())

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AUTH_SESSION_COOKIE_INSECURE"))
			Expect(err.Error()).To(ContainSubstring(production))
		})
	})
})
