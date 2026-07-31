package http

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/rest/pkg/mw"
	"github.com/ensoria/rest/pkg/rest"
)

// rejectingVerifier refuses every credential it is shown (test helper).
type rejectingVerifier struct{}

func (rejectingVerifier) Verify(*rest.Request) (*authkit.Principal, error) {
	return nil, errors.New("credential could not be verified")
}

func (rejectingVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

// anonymousVerifier reports that the request carried no credential (test helper).
type anonymousVerifier struct{}

func (anonymousVerifier) Verify(*rest.Request) (*authkit.Principal, error) {
	return nil, authkit.ErrNoCredential
}

func (anonymousVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

// chain composes the middlewares the way the pipeline does: the list runs
// outside-in, so it is applied in reverse (test helper).
func chain(middlewares []rest.Middleware, final rest.Handler) rest.Handler {
	next := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		next = middlewares[i](next)
	}
	return next
}

func request() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, "/things", nil))
}

var _ = Describe("globalMiddlewares", func() {
	settings := &mw.CORSSettings{AllowOrigin: "*"}
	panicResponse := &rest.Response{Code: http.StatusInternalServerError}

	// Accepting the verifier and then forgetting to install the middleware is a
	// silent hole: the application compiles and serves every request unchecked.
	It("refuses a request whose credential cannot be trusted", func() {
		reached := false

		res := chain(globalMiddlewares(settings, rejectingVerifier{}, panicResponse),
			func(*rest.Request) *rest.Response {
				reached = true
				return &rest.Response{Code: http.StatusOK}
			})(request())

		Expect(res.Code).To(Equal(http.StatusUnauthorized))
		Expect(reached).To(BeFalse(), "the request reached the handler without being authenticated")
	})

	It("keeps serving a request that presents no credential", func() {
		res := chain(globalMiddlewares(settings, anonymousVerifier{}, panicResponse),
			func(*rest.Request) *rest.Response { return &rest.Response{Code: http.StatusOK} })(request())

		Expect(res.Code).To(Equal(http.StatusOK),
			"a public endpoint must still be reachable without a credential")
	})
})
