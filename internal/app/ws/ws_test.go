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

		router := wsApp.CreateWSRouter(modules, rejectingVerifier{}, sameOriginOnly)

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
			wskit.Raw(wsconfig.NewDefaultModule("/ws/two")),
		}

		wsApp.CreateWSRouter(modules, rejectingVerifier{}, sameOriginOnly)

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
			wskit.Raw(wsconfig.NewDefaultModule("/ws/two")),
		}

		wsApp.CreateWSRouter(modules, allowingVerifier{}, middleware.ParseOrigins("https://app.example.test"))

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

		wsApp.CreateWSRouter(modules, rejectingVerifier{}, middleware.ParseOrigins(""))

		// Both guards would refuse this handshake. Whichever answers first is
		// the one that ran first.
		res := modules[0].RuntimeModule().HTTPMiddlewares[0](upgradeFrom("https://evil.example"))

		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusForbidden), "the credential check answered first")
	})
})
