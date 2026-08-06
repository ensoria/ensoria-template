package ws_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"
)

// rejectingVerifier refuses every credential it is shown (test helper).
type rejectingVerifier struct{}

func (rejectingVerifier) Verify(*rest.Request) (*authkit.Principal, error) {
	return nil, errors.New("credential could not be verified")
}

func (rejectingVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

func upgradeRequest() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, "/ws/things", nil))
}

var _ = Describe("CreateWSRouter", func() {
	It("keeps the modules it was given", func() {
		modules := []*wskit.Module{
			wskit.NewModule(&wskit.Channel{Path: "/ws/one"}),
			wskit.NewModule(&wskit.Channel{Path: "/ws/two"}),
		}

		router := wsApp.CreateWSRouter(modules, rejectingVerifier{})

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

		wsApp.CreateWSRouter(modules, rejectingVerifier{})

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
})
