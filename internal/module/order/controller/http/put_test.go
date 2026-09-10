package http_test

import (
	nethttp "net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	orderhttp "github.com/ensoria/ensoria-template/internal/module/order/controller/http"
	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/module/order/service/mock"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// alicesOrder is the order under test. It belongs to alice and to nobody else.
func alicesOrder() *dto.Order {
	return &dto.Order{Id: 1, OwnerSubject: "alice", Amount: 1000, Status: "pending"}
}

// caller holds orders:write — everything the scope declaration asks for. Who
// the caller is, is exactly what the specs below vary.
func caller(subject string) *authkit.Principal {
	return &authkit.Principal{
		Subject: subject,
		Scheme:  authkit.SchemeJWT,
		Scopes:  []string{"orders:write"},
	}
}

// update sends a replacement for order 1 against a service that answers with
// alice's order.
func update(subject string) *rest.Response {
	svc := mock.NewOrderServiceMock()
	svc.WillReturn("FindOrder", alicesOrder(), nil)
	svc.WillReturn("UpdateOrder", alicesOrder(), nil)

	return handle(svc, subject, `{"amount":1500,"status":"paid"}`)
}

func handle(svc service.OrderService, subject, body string) *rest.Response {
	ctrl := restkit.NewController(orderhttp.NewPut(svc))

	raw := httptest.NewRequest(nethttp.MethodPut, "/order/1", strings.NewReader(body))
	raw.Header.Set("Content-Type", "application/json")
	raw.SetPathValue("id", "1")
	r := rest.NewRequest(raw)
	if subject != "" {
		r.SetContext(authkit.WithPrincipal(r.Context(), caller(subject)))
	}
	return r2(ctrl, r)
}

// r2 runs the controller. It exists so that the helper above reads as one
// statement per step rather than ending in a nested call.
func r2(ctrl rest.Controller, r *rest.Request) *rest.Response {
	return ctrl.Handle(r)
}

// errorCode reads the code out of the shared error envelope.
func errorCode(res *rest.Response) string {
	GinkgoHelper()
	envelope, ok := res.Body.(*restkit.ErrorEnvelope)
	Expect(ok).To(BeTrue(), "the response body is not the shared error envelope")
	return envelope.Error.Code
}

var _ = Describe("PUT /order/{id}", func() {
	It("updates the order for the caller it belongs to", func() {
		res := update("alice")

		Expect(res.Code).To(Equal(nethttp.StatusOK))
	})

	// ⚠ This is the spec the whole example exists for. bob holds every scope
	// the endpoint declares — the credential is not the question — and the
	// order is not his.
	It("refuses a caller holding every scope, for somebody else's order", func() {
		res := update("bob")

		Expect(res.Code).To(Equal(nethttp.StatusForbidden))
		Expect(errorCode(res)).To(Equal(restkit.ForbiddenCode))
	})

	// The resource rule is not a substitute for the scope: a caller without
	// orders:write is refused before the order is ever read.
	It("refuses a caller without the scope, without reading the order", func() {
		svc := mock.NewOrderServiceMock()
		ctrl := restkit.NewController(orderhttp.NewPut(svc))

		raw := httptest.NewRequest(nethttp.MethodPut, "/order/1",
			strings.NewReader(`{"amount":1500,"status":"paid"}`))
		raw.Header.Set("Content-Type", "application/json")
		raw.SetPathValue("id", "1")
		r := rest.NewRequest(raw)
		r.SetContext(authkit.WithPrincipal(r.Context(),
			&authkit.Principal{Subject: "alice", Scheme: authkit.SchemeJWT, Scopes: []string{"orders:read"}}))

		res := ctrl.Handle(r)

		Expect(res.Code).To(Equal(nethttp.StatusForbidden))
		Expect(svc.WasCalled("FindOrder")).To(BeFalse(),
			"an unauthorized caller must not cause the order to be read")
	})

	It("refuses a caller with no credential", func() {
		Expect(handle(mock.NewOrderServiceMock(), "", `{"amount":1500,"status":"paid"}`).Code).
			To(Equal(nethttp.StatusUnauthorized))
	})

	// The order has to be read before it can be authorized, so a missing one is
	// answered before the rule ever runs — and answering with an error is not a
	// skipped authorization.
	It("answers 404 for an order that does not exist", func() {
		svc := mock.NewOrderServiceMock()
		svc.WillReturn("FindOrder", nil, service.ErrOrderNotFound)

		res := handle(svc, "alice", `{"amount":1500,"status":"paid"}`)

		Expect(res.Code).To(Equal(nethttp.StatusNotFound))
		Expect(errorCode(res)).To(Equal("order_not_found"))
	})

	// Authorization comes first, so a refused caller must not have changed
	// anything on their way out.
	It("does not update the order it refused", func() {
		svc := mock.NewOrderServiceMock()
		svc.WillReturn("FindOrder", alicesOrder(), nil)
		svc.WillReturn("UpdateOrder", alicesOrder(), nil)

		handle(svc, "bob", `{"amount":1500,"status":"paid"}`)

		Expect(svc.WasCalled("UpdateOrder")).To(BeFalse())
	})

	// Validation runs after the scope check and before the handler, so a body
	// that cannot be accepted never reaches the order either.
	It("refuses a body missing a required field", func() {
		res := handle(mock.NewOrderServiceMock(), "alice", `{"status":"paid"}`)

		Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
	})

	Describe("what it declares", func() {
		// The declaration is what the generated documentation and the runtime
		// check both read, so it is pinned rather than inferred.
		It("names the scope and the rule the scope cannot express", func() {
			ep := orderhttp.NewPut(mock.NewOrderServiceMock())

			Expect(ep.Security.Scopes).To(Equal([]string{"orders:write"}))
			Expect(ep.Security.Resource).NotTo(BeNil())
			Expect(ep.Security.Resource.Description).To(ContainSubstring("belongs to"))
		})
	})
})
