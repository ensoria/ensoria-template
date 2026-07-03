package order

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/module/order/controller/http"
	"github.com/ensoria/ensoria-template/internal/module/order/dto"
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

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[service.OrderService](service.NewOrderService),
		http.NewGet,
		dikit.AsHTTPModule(NewModule),
	})
}
