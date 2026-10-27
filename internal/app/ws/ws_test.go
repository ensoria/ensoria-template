package ws_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsrouter"
)

// rejectingVerifier refuses every credential it is shown (test helper).
type rejectingVerifier struct{}

func (rejectingVerifier) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return nil, errors.New("credential could not be verified")
}

func (rejectingVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

func upgradeRequest() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, "/ws/things", nil))
}

// mustRouter fails the spec when the router could not be built.
func mustRouter(router *wsrouter.Router, err error) *wsrouter.Router {
	GinkgoHelper()

	Expect(err).NotTo(HaveOccurred())
	return router
}

// sameOriginOnly is the same-origin deployment: no other origin is claimed.
var sameOriginOnly = middleware.ParseOrigins("")

// upgradeFrom builds a handshake a browser started from origin.
func upgradeFrom(origin string) *rest.Request {
	req := httptest.NewRequest(http.MethodGet, "/ws/things", nil)
	req.Header.Set("Origin", origin)
	return rest.NewRequest(req)
}

// allowingVerifier accepts every request as an anonymous one, so that a spec
// about origins is not answered by the credential check instead.
type allowingVerifier struct{}

func (allowingVerifier) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return &authkit.VerifyResult{}, nil
}

func (allowingVerifier) Schemes() []string { return []string{authkit.SchemeSession} }

var _ = Describe("CreateWSRouter", func() {
	It("keeps the modules it was given", func() {
		modules := []*wskit.Module{
			wskit.NewModule(&wskit.Channel{Path: "/ws/one"}),
			wskit.NewModule(&wskit.Channel{Path: "/ws/two"}),
		}

		// nil is the scope policy: these specs are about which handshakes are
		// refused, and no policy means a caller is judged on the scopes their
		// credential literally carries. That the policy reaches a connection at
		// all is fixed in internal/middleware (AuthUpgrade).
		router, err := wsApp.CreateWSRouter(modules, rejectingVerifier{}, sameOriginOnly, nil, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(router.Modules).To(HaveLen(2))
	})

	// Guarding here rather than in every module is what makes a WebSocket
	// endpoint added later authenticated by default. Losing this would leave a
	// new module reachable without any credential check.
	It("puts the credential check in front of every module's upgrade", func() {
		// A raw module goes through the same guard: skipping it for undocumented
		// channels would make "raw" mean "unauthenticated" too.
		modules := []*wskit.Module{
			wskit.NewModule(&wskit.Channel{Path: "/ws/one"}),
			wskit.Raw(wsconfig.NewDefaultModule("/ws/two"), nil),
		}

		mustRouter(wsApp.CreateWSRouter(modules, rejectingVerifier{}, sameOriginOnly, nil, nil))

		for _, module := range modules {
			m := module.RuntimeModule()
			Expect(m.HTTPMiddlewares).NotTo(BeEmpty(), "module %s has no upgrade guard", m.Path)

			var refused *rest.Response
			for _, mw := range m.HTTPMiddlewares {
				if res := mw(upgradeRequest()); res != nil {
					refused = res
				}
			}
			Expect(refused).NotTo(BeNil(), "module %s accepts an untrusted credential", m.Path)
			Expect(refused.Code).To(Equal(http.StatusUnauthorized))
		}
	})

	// The same argument as the credential check, for the other guard: a channel
	// added later must not be the one that forgot it. Losing this leaves that
	// channel open to cross-site WebSocket hijacking, which the cross-origin
	// check on the HTTP side cannot cover — an upgrade is a GET.
	It("puts the origin check in front of every module's upgrade", func() {
		modules := []*wskit.Module{
			wskit.NewModule(&wskit.Channel{Path: "/ws/one"}),
			wskit.Raw(wsconfig.NewDefaultModule("/ws/two"), nil),
		}

		mustRouter(wsApp.CreateWSRouter(modules, allowingVerifier{}, middleware.ParseOrigins("https://app.example.test"), nil, nil))

		for _, module := range modules {
			m := module.RuntimeModule()

			var refused *rest.Response
			for _, mw := range m.HTTPMiddlewares {
				if res := mw(upgradeFrom("https://evil.example")); res != nil {
					refused = res
					break
				}
			}
			Expect(refused).NotTo(BeNil(), "module %s accepts a handshake from any origin", m.Path)
			Expect(refused.Code).To(Equal(http.StatusForbidden))
		}
	})

	// ⚠ Order, not merely presence. The origin check has to run first so that a
	// forged handshake is refused before the session store is asked about the
	// cookie the browser attached to it.
	It("checks the origin before it checks the credential", func() {
		modules := []*wskit.Module{wskit.NewModule(&wskit.Channel{Path: "/ws/one"})}

		mustRouter(wsApp.CreateWSRouter(modules, rejectingVerifier{}, middleware.ParseOrigins(""), nil, nil))

		// Both guards would refuse this handshake. Whichever answers first is
		// the one that ran first.
		res := modules[0].RuntimeModule().HTTPMiddlewares[0](upgradeFrom("https://evil.example"))

		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusForbidden), "the credential check answered first")
	})
})

// verifierFor reports the credential kinds a verifier can check, and treats
// every handshake as anonymous. The startup checks only ask what it can verify.
type verifierFor []string

func (verifierFor) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return &authkit.VerifyResult{}, nil
}

func (v verifierFor) Schemes() []string { return v }

// runGuards runs a module's pre-upgrade chain the way the WebSocket server
// does: in order, stopping at the first refusal.
func runGuards(module *wskit.Module, r *rest.Request) *rest.Response {
	for _, mw := range module.RuntimeModule().HTTPMiddlewares {
		if res := mw(r); res != nil {
			return res
		}
	}
	return nil
}

var _ = Describe("CreateWSRouter and the channel's declaration", func() {
	sessionsOnly := verifierFor{authkit.SchemeSession}

	// The default is closed, as it is for an endpoint: a channel added later
	// without a declaration must not be one anybody can open.
	It("refuses an anonymous handshake to a channel that declares nothing, with 401", func() {
		module := wskit.NewModule(&wskit.Channel{Path: "/ws/one"})
		mustRouter(wsApp.CreateWSRouter([]*wskit.Module{module}, sessionsOnly, sameOriginOnly, nil, nil))

		res := runGuards(module, upgradeRequest())

		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusUnauthorized))
	})

	It("lets an anonymous handshake through to a public channel", func() {
		module := wskit.NewModule(&wskit.Channel{Path: "/ws/one", Security: &wskit.SecuritySpec{Public: true}})
		mustRouter(wsApp.CreateWSRouter([]*wskit.Module{module}, sessionsOnly, sameOriginOnly, nil, nil))

		Expect(runGuards(module, upgradeRequest())).To(BeNil())
	})

	// "Raw" describes the framing, not the access: the same declaration and
	// the same default hold.
	It("judges a raw channel by the declaration it was given, closed when it was given none", func() {
		closed := wskit.Raw(wsconfig.NewDefaultModule("/ws/closed"), nil)
		public := wskit.Raw(wsconfig.NewDefaultModule("/ws/public"), &wskit.SecuritySpec{Public: true})
		mustRouter(wsApp.CreateWSRouter([]*wskit.Module{closed, public}, sessionsOnly, sameOriginOnly, nil, nil))

		res := runGuards(closed, upgradeRequest())
		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusUnauthorized))
		Expect(runGuards(public, upgradeRequest())).To(BeNil())
	})

	// ⚠ The guards have to run before anything the channel added itself. A
	// middleware a channel puts in front of its upgrade — one that looks the
	// room up, say — must never run for a handshake that is going to be
	// refused, or it would act for a caller nobody verified.
	It("runs the guards before the channel's own pre-upgrade middlewares", func() {
		ranChannelMiddleware := false
		module := wskit.NewModule(&wskit.Channel{
			Path: "/ws/one",
			Configure: func(m *wsconfig.Module) {
				m.AddHTTPMiddleware(func(*rest.Request) *rest.Response {
					ranChannelMiddleware = true
					return nil
				})
			},
		})
		mustRouter(wsApp.CreateWSRouter([]*wskit.Module{module}, rejectingVerifier{}, sameOriginOnly, nil, nil))

		res := runGuards(module, upgradeRequest())

		Expect(res).NotTo(BeNil())
		Expect(ranChannelMiddleware).To(BeFalse(), "the channel's middleware ran before the credential was checked")
	})

	It("still runs the channel's own middlewares for a handshake the guards let through", func() {
		ranChannelMiddleware := false
		module := wskit.NewModule(&wskit.Channel{
			Path:     "/ws/one",
			Security: &wskit.SecuritySpec{Public: true},
			Configure: func(m *wsconfig.Module) {
				m.AddHTTPMiddleware(func(*rest.Request) *rest.Response {
					ranChannelMiddleware = true
					return nil
				})
			},
		})
		mustRouter(wsApp.CreateWSRouter([]*wskit.Module{module}, sessionsOnly, sameOriginOnly, nil, nil))

		Expect(runGuards(module, upgradeRequest())).To(BeNil())
		Expect(ranChannelMiddleware).To(BeTrue())
	})

	Describe("the startup checks", func() {
		It("refuses a channel that needs a caller in an application that can verify none", func() {
			_, err := wsApp.CreateWSRouter([]*wskit.Module{wskit.NewModule(&wskit.Channel{Path: "/ws/one"})}, verifierFor{}, sameOriginOnly, nil, nil)

			Expect(err).To(MatchError(ContainSubstring("/ws/one")))
		})

		It("accepts public channels in an application that can verify nothing", func() {
			module := wskit.NewModule(&wskit.Channel{Path: "/ws/one", Security: &wskit.SecuritySpec{Public: true}})

			_, err := wsApp.CreateWSRouter([]*wskit.Module{module}, verifierFor{}, sameOriginOnly, nil, nil)

			Expect(err).NotTo(HaveOccurred())
		})

		// Requiring a kind of credential nothing verifies closes the channel
		// to everyone.
		It("refuses a channel that accepts only a kind of credential nothing verifies", func() {
			module := wskit.NewModule(&wskit.Channel{Path: "/ws/one", Security: &wskit.SecuritySpec{Schemes: []string{authkit.SchemeAPIKey}}})

			_, err := wsApp.CreateWSRouter([]*wskit.Module{module}, sessionsOnly, sameOriginOnly, nil, nil)

			Expect(err).To(MatchError(ContainSubstring(authkit.SchemeAPIKey)))
			Expect(err).To(MatchError(ContainSubstring("/ws/one")))
		})

		// A public channel admits everyone before its conditions are read, so
		// they would never apply: it fails open.
		DescribeTable("refuses a public channel that also states a condition",
			func(security *wskit.SecuritySpec) {
				module := wskit.NewModule(&wskit.Channel{Path: "/ws/one", Security: security})

				_, err := wsApp.CreateWSRouter([]*wskit.Module{module}, sessionsOnly, sameOriginOnly, nil, nil)

				Expect(err).To(MatchError(ContainSubstring("/ws/one")))
				Expect(err).To(MatchError(ContainSubstring("drop Public")))
			},
			Entry("a scope", &wskit.SecuritySpec{Public: true, Scopes: []string{"things:read"}}),
			Entry("a scheme", &wskit.SecuritySpec{Public: true, Schemes: []string{authkit.SchemeSession}}),
		)

		It("checks raw channels the same way", func() {
			_, err := wsApp.CreateWSRouter([]*wskit.Module{wskit.Raw(wsconfig.NewDefaultModule("/ws/raw"), nil)}, verifierFor{}, sameOriginOnly, nil, nil)

			Expect(err).To(MatchError(ContainSubstring("/ws/raw")))
		})
	})
})
