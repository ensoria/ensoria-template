package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// NewGet は現在の注文を取得するエンドポイント(型付き Endpoint)。
// GET はボディが無いので Req 型は restkit.NoBody。レスポンスは dto.Order(構造体)なので
// docai のレスポンス表・example は型から自動生成される。
func NewGet(svc service.OrderService) *restkit.Endpoint[restkit.NoBody, dto.Order] {
	return &restkit.Endpoint[restkit.NoBody, dto.Order]{
		Summary:  "Fetch the current order",
		Task:     "read order",
		IDPrefix: "ord",
		Success:  http.StatusOK,
		FieldDocs: map[string]string{
			"amount": "Order total amount",
			"status": "Order lifecycle status",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.Order], error) {
			order, err := svc.GetOrder()
			if err != nil {
				// ただの error を返すと 500 internal_error に丸められる(内部詳細を漏らさない)。
				// 特定のステータス/コードで返したい場合は HTTPError を返す。
				// 例: 不正な注文リクエストとして 400 を返す。
				return nil, restkit.NewError(http.StatusBadRequest, "invalid_order", "could not fetch the order")
			}
			return rest.NewResult(order), nil
		},
	}
}
