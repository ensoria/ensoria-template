package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// NewPaymentCallback は決済事業者が決済結果を通知してくるエンドポイント(型付き Endpoint)。
//
// このエンドポイントは `Schemes` の使い方の実例。呼び出すのは決済事業者のサーバであって、
// ブラウザの向こうにいる利用者ではない。受け付ける資格情報を API キーだけに絞ると、
// 利用者のログイントークンでは呼べなくなる —— たとえそのトークンが必要なスコープを
// 持っていたとしても。`Scopes` が「何ができるか」を制限するのに対し、`Schemes` は
// 「どういう種類の呼び出し元か」を制限する。
//
// 応答は 204 No Content。返す本文が無いので Res 型は restkit.NoBody。
func NewPaymentCallback(svc service.OrderService) *restkit.Endpoint[dto.PaymentCallback, restkit.NoBody] {
	return &restkit.Endpoint[dto.PaymentCallback, restkit.NoBody]{
		Summary:  "Record the outcome of a payment",
		Task:     "record a payment outcome",
		IDPrefix: "ord",
		Success:  http.StatusNoContent,
		Security: &restkit.SecuritySpec{
			Schemes: []string{authkit.SchemeAPIKey},
			Scopes:  []string{"orders:write"},
		},
		BodyRules: []*rule.RuleSet{
			{Field: "paymentId", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(64)}},
			{Field: "status", Rules: []rule.Rule{vkit.Required()}},
		},
		FieldDocs: map[string]string{
			"paymentId": "Identifier assigned by the payment provider, not by this API",
			"orderId":   "Order the payment settles",
			"status":    "Outcome reported by the provider (settled, failed)",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"marks the order as paid"},
			// 事業者は同じ通知を再送してくる。同じ paymentId を二度受けても結果は同じ。
			Idempotent: new(true),
		},
		Related: []string{
			"Read the resulting order: GET /order",
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         "order_not_found",
				Condition:    "No order exists under orderId",
				CallerAction: "Do not retry; the notification refers to an unknown order.",
			},
		},
		Handle: func(r *rest.Request, req *dto.PaymentCallback) (*rest.Result[restkit.NoBody], error) {
			// ここで svc.ApplyPayment(req) を呼ぶ
			_ = svc

			return restkit.NoContent(), nil
		},
	}
}
