package userpost

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/query/user_post/controller/http"
	"github.com/ensoria/ensoria-template/internal/query/user_post/dto"
	"github.com/ensoria/ensoria-template/internal/query/user_post/repository"
	"github.com/ensoria/ensoria-template/internal/query/user_post/service"
	"github.com/ensoria/rest/pkg/rest"
)

const ModuleName = "user_post"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

func NewModule(get *restkit.Endpoint[restkit.NoBody, dto.GetUserPost]) *rest.Module {
	return &rest.Module{
		Path: "/users/{id}/posts",
		Get:  restkit.NewController(get),
	}
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[repository.UserPostRepository](repository.NewUserPostRepository),
		dikit.ProvideAs[service.UserPostService](service.NewUserPostService),
		http.NewGet,
		dikit.AsHTTPModule(NewModule),
	})
}
