package http_test

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"

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

// approver holds orders:write and whatever approval limit a spec gives it. The
// limit is untyped on purpose: half of these specs are about what happens when
// it is not the type the rule expects.
func approver(limit any) *authkit.Principal {
	p := &authkit.Principal{
		Subject: "staff",
		Scheme:  authkit.SchemeJWT,
		Scopes:  []string{"orders:write"},
	}
	if limit != nil {
		p.Claims = authkit.Claims{"approval_limit": limit}
	}
	return p
}

// approvingOrder is the service the approval specs run against: it finds alice's
// order (1000) and reports back whatever it was asked to save.
func approvingOrder() *mock.OrderServiceMock {
	svc := mock.NewOrderServiceMock()
	svc.WillReturn("FindOrder", alicesOrder(), nil)
	svc.WillReturn("UpdateOrder", &dto.Order{
		Id: 1, OwnerSubject: "alice", Amount: 1000, Status: "approved",
	}, nil)
	return svc
}

// approve posts the approval as the given caller. A nil principal is a request
// carrying no credential.
func approve(svc service.OrderService, principal *authkit.Principal) *rest.Response {
	ctrl := restkit.NewController(orderhttp.NewApprove(svc))

	raw := httptest.NewRequest(nethttp.MethodPost, "/order/1/approval", nil)
	raw.SetPathValue("id", "1")
	r := rest.NewRequest(raw)
	if principal != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), principal))
	}
	return ctrl.Handle(r)
}

var _ = Describe("POST /order/{id}/approval", func() {
	// The order costs 1000, and this caller may approve up to 2000.
	It("approves an order the caller's limit covers", func() {
		svc := approvingOrder()

		res := approve(svc, approver(float64(2000)))

		Expect(res.Code).To(Equal(nethttp.StatusOK))
		Expect(svc.WasCalled("UpdateOrder")).To(BeTrue())
	})

	// ⚠ The spec this endpoint exists for. The caller holds every scope the
	// endpoint declares, and is refused by what their token says about them.
	It("refuses a caller holding every scope, for an order above their limit", func() {
		svc := approvingOrder()

		res := approve(svc, approver(float64(500)))

		Expect(res.Code).To(Equal(nethttp.StatusForbidden))
		Expect(errorCode(res)).To(Equal(restkit.ForbiddenCode))
		Expect(svc.WasCalled("UpdateOrder")).To(BeFalse(),
			"a refused caller must not have changed the order")
	})

	// No limit is not an unlimited limit. A rule that read a missing claim as
	// zero-and-carry-on would approve nothing; one that ignored it would approve
	// everything, which is the failure worth being certain about.
	It("refuses a caller whose token carries no approval limit", func() {
		svc := approvingOrder()

		res := approve(svc, approver(nil))

		Expect(res.Code).To(Equal(nethttp.StatusForbidden))
		Expect(svc.WasCalled("UpdateOrder")).To(BeFalse())
	})

	// A number written as a string is a different claim, and reading it as a
	// number would be this application guessing at what the issuer meant. The
	// fix is a mapper in the identity provider, and a 403 is how it is found.
	It("refuses a limit the identity provider wrote as a string", func() {
		svc := approvingOrder()

		res := approve(svc, approver("2000"))

		Expect(res.Code).To(Equal(nethttp.StatusForbidden))
		Expect(svc.WasCalled("UpdateOrder")).To(BeFalse())
	})

	// The same claim reaches a principal in whichever type its source used: JSON
	// numbers from a token, a json.Number from a decoder that keeps digits, a Go
	// int from a KeyStore. All of them are the same limit.
	DescribeTable("reads the limit however the credential wrote it",
		func(limit any) {
			res := approve(approvingOrder(), approver(limit))

			Expect(res.Code).To(Equal(nethttp.StatusOK))
		},
		Entry("a JSON number, as a verified token gives it", float64(2000)),
		Entry("a json.Number, as a decoder keeping digits gives it", json.Number("2000")),
		Entry("a Go int, as a KeyStore building a caller gives it", 2000),
	)

	// The scope is judged before anything is read: a caller who may not write
	// orders at all never reaches the order, let alone the rule about it.
	It("refuses a caller without the scope, without reading the order", func() {
		svc := approvingOrder()

		res := approve(svc, &authkit.Principal{
			Subject: "staff",
			Scheme:  authkit.SchemeJWT,
			Scopes:  []string{"orders:read"},
			Claims:  authkit.Claims{"approval_limit": float64(2000)},
		})

		Expect(res.Code).To(Equal(nethttp.StatusForbidden))
		Expect(svc.WasCalled("FindOrder")).To(BeFalse(),
			"an unauthorized caller must not cause the order to be read")
	})

	It("refuses a caller with no credential", func() {
		res := approve(approvingOrder(), nil)

		Expect(res.Code).To(Equal(nethttp.StatusUnauthorized))
	})

	// The order is read before the rule can be about it, so an id nobody owns is
	// a 404 rather than a 403 — there is no resource for the rule to judge.
	It("reports an order that does not exist", func() {
		svc := mock.NewOrderServiceMock()
		svc.WillReturn("FindOrder", nil, service.ErrOrderNotFound)

		res := approve(svc, approver(float64(2000)))

		Expect(res.Code).To(Equal(nethttp.StatusNotFound))
		Expect(errorCode(res)).To(Equal("order_not_found"))
	})

	// The approval keeps the amount it approved: UpdateOrder replaces the order,
	// so an approval that dropped the amount would zero every order it touched.
	It("keeps the order's amount when it approves it", func() {
		svc := approvingOrder()

		Expect(approve(svc, approver(float64(2000))).Code).To(Equal(nethttp.StatusOK))

		args := svc.GetLastCallArgs("UpdateOrder")
		Expect(args).To(HaveLen(2))
		update, ok := args[1].(*dto.UpdateOrder)
		Expect(ok).To(BeTrue())
		Expect(update.Status).To(Equal("approved"))
		Expect(update.Amount).NotTo(BeNil())
		Expect(*update.Amount).To(Equal(float64(1000)))
	})
})
