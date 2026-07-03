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
		IDPrefix: "ord",
		Success:  http.StatusOK,
		FieldDocs: map[string]string{
			"amount": "Order total amount",
			"status": "Order lifecycle status",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  boolPtr(true),
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.Order], error) {
			order, err := svc.GetOrder()
			if err != nil {
				return nil, err
			}
			return rest.NewResult(order), nil
		},
	}
}

func boolPtr(b bool) *bool { return &b }
