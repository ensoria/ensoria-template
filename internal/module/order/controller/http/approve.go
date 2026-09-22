package http

import (
	"errors"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// approvalLimitClaim is the claim carrying how much the caller may approve.
// It is the identity provider that asserts it — this application only reads it.
const approvalLimitClaim = "approval_limit"

// approvedStatus is where an approved order lands in its lifecycle.
const approvedStatus = "approved"

// orderApprovalCheck is the resource rule of POST /order/{id}/approval, and the
// example of a rule decided by something the *credential* carried rather than by
// who the caller is.
//
// The claim is read here, in the declaration, and the domain is handed the value
// it reasons about — the same split orderOwnerCheck makes with p.Subject. What
// "covered by the approval limit" means belongs to orders
// (service.WithinApprovalLimit); what a claim is, and how to read one, belongs
// to the authorization layer.
//
// ⚠ A caller whose token carries no approval_limit is refused, and so is one
// whose token carries it as the string "1500" rather than as a number. Both are
// the same answer — this caller has not been given an approval limit — and
// Claims.Number is what makes them the same answer instead of a panic, a
// silent zero, or a guess about what "1500" was supposed to mean. Under
// AUTH_MODE=jwks that is a mapper to fix in the identity provider; the realm in
// .keycloak shows one.
var orderApprovalCheck = restkit.NewResourceCheck[dto.Order](
	"Only a caller whose approval limit covers the order's amount can approve it.",
	func(p *authkit.Principal, order *dto.Order) bool {
		limit, ok := p.Claims.Number(approvalLimitClaim)
		return ok && service.WithinApprovalLimit(limit, order)
	},
)

// NewApprove approves one order.
//
// It is the second half of the resource-authorization example. PUT /order/{id}
// is refused on who the caller is (the order is somebody else's); this one is
// refused on what their credential says they may do — and neither question can
// be answered before the order has been read, which is why both rules live on
// the declaration and run inside the handler.
//
// The two are deliberately separate endpoints. An endpoint needing two genuinely
// different rules is asking to be two endpoints, and approving somebody else's
// order is the normal case here rather than the exception: approval is a staff
// action, so ownership is not part of it.
func NewApprove(svc service.OrderService) *restkit.Endpoint[restkit.NoBody, dto.Order] {
	return &restkit.Endpoint[restkit.NoBody, dto.Order]{
		Summary:     "Approve an order",
		Description: "Marks an order approved, if the caller's approval limit covers its amount.",
		Task:        "approve order",
		IDPrefix:    "ord",
		Success:     http.StatusOK,
		Security: &restkit.SecuritySpec{
			// The scope answers "may approve orders at all". Whether this
			// order is within reach is the claim's answer, below.
			Scopes:   []string{"orders:write"},
			Resource: orderApprovalCheck,
		},
		PathRules: []*rule.RuleSet{
			{Field: "id", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}},
		},
		FieldDocs: map[string]string{
			"id":           "Order identifier",
			"ownerSubject": "Caller the order belongs to",
			"amount":       "Order total amount",
			"status":       "Order lifecycle status",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"marks the order as approved"},
			// Approving an approved order leaves it approved.
			Idempotent: new(true),
			Preconditions: []string{
				"the order exists",
				"the caller's approval limit covers the order's amount",
			},
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         "order_not_found",
				Condition:    "No order exists under that id",
				CallerAction: "Do not retry; check the id.",
			},
			{
				Status:    http.StatusForbidden,
				Code:      restkit.ForbiddenCode,
				Condition: "The caller's approval limit does not cover the order, or they have none",
				CallerAction: "Do not retry. A higher limit is granted by the identity provider, " +
					"not by this API.",
			},
		},
		Related: []string{
			"Update an order: PUT /order/{id}",
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.Order], error) {
			id, err := orderID(r)
			if err != nil {
				return nil, err
			}

			order, err := svc.FindOrder(id)
			if err != nil {
				if errors.Is(err, service.ErrOrderNotFound) {
					return nil, restkit.NewError(
						http.StatusNotFound, "order_not_found", "no order exists under that id")
				}
				return nil, err
			}

			// Authorize as soon as the order exists: the rule is about this
			// order's amount, so there is nothing to decide before it is read
			// and nothing to change before it is decided.
			if err := restkit.Authorize(r, order); err != nil {
				return nil, err
			}

			// The amount is carried over rather than left out: UpdateOrder
			// replaces the order, and a missing amount would clear it.
			amount := order.Amount
			approved, err := svc.UpdateOrder(id, &dto.UpdateOrder{Amount: &amount, Status: approvedStatus})
			if err != nil {
				return nil, err
			}
			return rest.NewResult(approved), nil
		},
	}
}
