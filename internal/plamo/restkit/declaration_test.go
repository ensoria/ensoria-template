package restkit_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// getRequest builds a bodyless request for the controllers under test.
func getRequest() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, "/things", nil))
}

// returning builds a controller whose handler answers with the given status.
func returning(status int, responses ...restkit.ResponseSpec) rest.Controller {
	ep := &restkit.Endpoint[restkit.NoBody, okBody]{
		Success:   http.StatusOK,
		Responses: responses,
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[okBody], error) {
			return rest.NewResult(&okBody{OK: true}, rest.WithStatus(status)), nil
		},
	}
	return restkit.NewController(ep)
}

type okBody struct {
	OK bool `json:"ok"`
}

// A status the handler returns must be declared, otherwise the generated
// documentation silently drifts from the implementation. The declaration is
// therefore load-bearing: the adapter checks it on every response.
var _ = Describe("undeclared success status", func() {
	AfterEach(func() { restkit.SetStrictDeclarations(false) })

	Describe("in strict mode (local / test / development)", func() {
		BeforeEach(func() { restkit.SetStrictDeclarations(true) })

		It("panics when the handler returns a status nobody declared", func() {
			ctrl := returning(http.StatusCreated)

			Expect(func() { ctrl.Handle(getRequest()) }).
				To(PanicWith(ContainSubstring("201")))
		})

		It("accepts the declared primary success status", func() {
			ctrl := returning(http.StatusOK)

			Expect(ctrl.Handle(getRequest()).Code).To(Equal(http.StatusOK))
		})

		It("accepts a status declared through Responses", func() {
			ctrl := returning(http.StatusCreated, restkit.ResponseSpec{
				Status: http.StatusCreated, When: "a new user was created",
			})

			Expect(ctrl.Handle(getRequest()).Code).To(Equal(http.StatusCreated))
		})

		It("accepts the implicit 200 when Success is left unset", func() {
			ep := &restkit.Endpoint[restkit.NoBody, okBody]{
				Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[okBody], error) {
					return rest.NewResult(&okBody{OK: true}), nil
				},
			}

			Expect(restkit.NewController(ep).Handle(getRequest()).Code).To(Equal(http.StatusOK))
		})
	})

	Describe("StrictForEnv", func() {
		It("is strict only in the environments a developer works in", func() {
			Expect(restkit.StrictForEnv("local")).To(BeTrue())
			Expect(restkit.StrictForEnv("test")).To(BeTrue())
			Expect(restkit.StrictForEnv("development")).To(BeTrue())
			Expect(restkit.StrictForEnv("staging")).To(BeFalse())
			Expect(restkit.StrictForEnv("production")).To(BeFalse())
			Expect(restkit.StrictForEnv("")).To(BeFalse())
		})
	})

	Describe("outside strict mode (staging / production)", func() {
		It("still answers with the undeclared status instead of failing the request", func() {
			ctrl := returning(http.StatusCreated)

			var res *rest.Response
			Expect(func() { res = ctrl.Handle(getRequest()) }).NotTo(Panic())
			Expect(res.Code).To(Equal(http.StatusCreated))
		})
	})
})
