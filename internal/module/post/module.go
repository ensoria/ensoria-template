package post

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	postgrpc "github.com/ensoria/ensoria-template/internal/module/post/controller/grpc"
	"github.com/ensoria/ensoria-template/internal/module/post/controller/http"
	"github.com/ensoria/ensoria-template/internal/module/post/dto"
	"github.com/ensoria/ensoria-template/internal/module/post/service"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	pb "github.com/ensoria/ensoria-template/pb/post"
	"github.com/ensoria/rest/pkg/rest"
)

const ModuleName = "post"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

func NewModule(get *restkit.Endpoint[restkit.NoBody, dto.Post]) *rest.Module {
	return &rest.Module{
		Path: "/post",
		Get:  restkit.NewController(get),
	}
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[service.PostService](service.NewPostService),
		http.NewGet,
		dikit.AsHTTPModule(NewModule),
		// gRPC
		dikit.AsGRPCService(postgrpc.NewPostGRPCService),
		dikit.ProvideAs[pb.PostServer](postgrpc.NewPostGRPCService),
	})
}
