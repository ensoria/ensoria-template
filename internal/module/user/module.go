package user

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	usergrpc "github.com/ensoria/ensoria-template/internal/module/user/controller/grpc"
	"github.com/ensoria/ensoria-template/internal/module/user/controller/http"
	usermb "github.com/ensoria/ensoria-template/internal/module/user/controller/mb"
	"github.com/ensoria/ensoria-template/internal/module/user/controller/ws"
	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/job"
	"github.com/ensoria/ensoria-template/internal/module/user/repository"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/module/user/task"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"

	"github.com/ensoria/ensoria-template/internal/infra/grpcclt"
	pbPost "github.com/ensoria/ensoria-template/pb/post"
	pb "github.com/ensoria/ensoria-template/pb/user"
)

const ModuleName = "user"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

// rest
func NewUserByIDModule(
	get *restkit.Endpoint[restkit.NoBody, dto.GetUser],
	patch *restkit.Endpoint[dto.UpdateUser, dto.GetUser],
) *rest.Module {
	return &rest.Module{
		Path:  "/users/{id}",
		Get:   restkit.NewController(get),
		Patch: restkit.NewController(patch),
	}
}

func NewUserCollectionModule(post *restkit.Endpoint[dto.CreateUser, dto.CreateUser]) *rest.Module {
	return &rest.Module{
		Path: "/users",
		Post: restkit.NewController(post),
	}
}

// websocket
//
// 経路の宣言(パス・メッセージカタログ)はcontroller側のChannelが持つ。
// wskitがそこから実行時モジュールを組み立て、受信ディスパッチを差し込む。
func NewWebSocketModule(channel *ws.Channel) *wskit.Module {
	return wskit.NewModule(channel.Declare())
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[repository.UserRepository](repository.NewUserRepository),
		dikit.ProvideAs[service.UserService](service.NewUserService),
		http.NewGet,
		http.NewPost,
		http.NewPatch,
		dikit.AsHTTPModule(NewUserByIDModule),
		dikit.AsHTTPModule(NewUserCollectionModule),

		// WebSocket
		ws.NewOnOpen,
		ws.NewChannel,
		dikit.AsWSModule(NewWebSocketModule),

		// gRPC server
		dikit.AsGRPCService(usergrpc.NewUserGRPCService),
		dikit.ProvideAs[pb.UserServer](usergrpc.NewUserGRPCService),

		// MB subscriptions: 宣言をgroupへ登録すると、app層のinvocationが
		// まとめて購読を開始する（モジュールごとの起動用invocationは不要）。
		dikit.AsMBSubscription(usermb.NewHelloWorldSubscription),

		// MB publications: 型付きのPublicationをserviceやcontrollerへ直接注入しつつ、
		// 同じ宣言をdescribe用のgroupにも登録する。
		usermb.NewHelloWorldPublication,
		dikit.AsMBPublication(mbkit.AsPublicationDoc[dto.HelloWorld]),
		usermb.NewUserCreatedPublication,
		dikit.AsMBPublication(mbkit.AsPublicationDoc[dto.UserCreated]),

		// gRPC client
		// 別のgRPCサーバーのクライアントが必要な場合は、コンストラクタを追加
		// このコンストラクタが必要な`grpc.ClientConnInterface`は、`service/connection`で定義する
		// gRPCクライアントのコンストラクタは、`dikit.InjectNamed`を使って、どの
		// gRPCコネクションを使うかを指定すること
		dikit.InjectGRPCClient(pbPost.NewPostClient, grpcclt.PostConnName),

		// worker jobs
		job.NewSimpleJob,
		dikit.AsWorkerJob(job.NewUserJob),

		// scheduler tasks
		task.NewSimpleTask,
		dikit.AsScheduledTask(task.NewUserTask),
	})
}
