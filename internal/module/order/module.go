package order

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/module/order/controller/http"
	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/repository"
	"github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

const ModuleName = "order"

// approvalEndpointName tells the approval endpoint apart from GET /order in the
// container: both are Endpoint[NoBody, dto.Order], and a type is all the
// container has to go on.
const approvalEndpointName = "orderApprove"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

func NewModule(get *restkit.Endpoint[restkit.NoBody, dto.Order]) *rest.Module {
	return &rest.Module{
		Path: "/order",
		Get:  restkit.NewController(get),
	}
}

func NewPaymentCallbackModule(callback *restkit.Endpoint[dto.PaymentCallback, restkit.NoBody]) *rest.Module {
	return &rest.Module{
		Path: "/order/payment-callback",
		Post: restkit.NewController(callback),
	}
}

// NewOrderByIDModule は id で指す1件の注文。
//
// `/order/payment-callback` より後に宣言していても構わない —— ルーティングは
// net/http の ServeMux が行い、リテラルのパスがテンプレートより優先されるため、
// `/order/payment-callback` が `/order/{id}` に吸われることはない。
func NewOrderByIDModule(put *restkit.Endpoint[dto.UpdateOrder, dto.Order]) *rest.Module {
	return &rest.Module{
		Path: "/order/{id}",
		Put:  restkit.NewController(put),
	}
}

// NewOrderApprovalModule は1件の注文の承認。
//
// 承認は「注文の更新」とは別のルールで守られる(呼び出し元の承認限度額)。
// 別のルールは別のエンドポイントにする —— 1つの宣言に2つのルールを持たせない。
func NewOrderApprovalModule(approve *restkit.Endpoint[restkit.NoBody, dto.Order]) *rest.Module {
	return &rest.Module{
		Path: "/order/{id}/approval",
		Post: restkit.NewController(approve),
	}
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[repository.OrderRepository](repository.NewOrderRepository),
		dikit.ProvideAs[service.OrderService](service.NewOrderService),
		http.NewGet,
		http.NewPut,
		dikit.ProvideNamed(http.NewApprove, approvalEndpointName),
		http.NewPaymentCallback,
		dikit.AsHTTPModule(NewModule),
		dikit.AsHTTPModule(NewOrderByIDModule),
		dikit.AsHTTPModuleWithTags(NewOrderApprovalModule, `name:"`+approvalEndpointName+`"`),
		dikit.AsHTTPModule(NewPaymentCallbackModule),
	})
}
