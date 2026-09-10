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

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[repository.OrderRepository](repository.NewOrderRepository),
		dikit.ProvideAs[service.OrderService](service.NewOrderService),
		http.NewGet,
		http.NewPut,
		http.NewPaymentCallback,
		dikit.AsHTTPModule(NewModule),
		dikit.AsHTTPModule(NewOrderByIDModule),
		dikit.AsHTTPModule(NewPaymentCallbackModule),
	})
}
