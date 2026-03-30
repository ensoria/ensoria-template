package user

import (
	"context"
	"log/slog"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	usergrpc "github.com/ensoria/ensoria-template/internal/module/user/controller/grpc"
	"github.com/ensoria/ensoria-template/internal/module/user/controller/http"
	usermb "github.com/ensoria/ensoria-template/internal/module/user/controller/mb"
	"github.com/ensoria/ensoria-template/internal/module/user/controller/ws"
	"github.com/ensoria/ensoria-template/internal/module/user/job"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/module/user/task"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"

	"github.com/ensoria/ensoria-template/internal/infra/grpcclt"
	pbPost "github.com/ensoria/ensoria-template/pb/post"
	pb "github.com/ensoria/ensoria-template/pb/user"
)

// TODO: encliでモジュールを作成したら、このファイルに
// 自動的に、NewModuleと、Constructorsを追加する
// さらに、moduler.goにもimportを追加すること

const ModuleName = "user"

// TODO: 便利機能として、この関数も自動的にencliで生成する
func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

// rest
func NewModule(get *http.Get, post *http.Post) *rest.Module {
	return &rest.Module{
		Path: "/user",
		Get:  get,
		Post: post,
	}
}

// websocket
func NewWebSocketModule(onOpen *ws.OnOpen, onMessage *ws.OnMessage) *wsconfig.Module {
	module := wsconfig.NewDefaultModule("/ws/" + ModuleName)
	// for logging
	module.AddOnOpenMiddleware(ws.LogOnOpen)
	module.OnOpen = onOpen.OnOpen()

	// for logging
	module.AddOnMessageMiddleware(ws.LogOnMessage)
	module.OnMessage = onMessage.OnMessage()
	return module
}

// メッセージブローカーSubscriberは戻り値がないため、injectされることもない。そのため、constructorでは起動させることができない。
// 登録はconstructorsではなく、invocationsに登録する必要がある
func NewSubscribeModule(lc dikit.LC, subscribe mb.StartSubscription, handler mb.SubscribeHandler) {
	onStart := func(ctx context.Context) error {
		slog.Info("start subscribing to hello_world")
		return subscribe("hello_world", handler,
			mb.WithErrorStrategy(mb.ErrorStrategyDiscard),
		)
	}
	// Subscriberは、onStartを定義したら、RegisterMBSubscriberOnStartLifecycleに登録する
	dikit.RegisterOnStartLifecycle(lc, onStart)
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.ProvideAs[service.UserService](service.NewUserService),
		http.NewGet,
		http.NewPost,
		dikit.AsHTTPModule(NewModule),

		// WebSocket
		ws.NewOnOpen,
		ws.NewOnMessage,
		dikit.AsWSModule(NewWebSocketModule),

		// gRPC server
		dikit.AsGRPCService(usergrpc.NewUserGRPCService),
		dikit.ProvideAs[pb.UserServer](usergrpc.NewUserGRPCService),

		// MB Subscriber
		dikit.ProvideAsNamed[mb.SubscribeHandler](usermb.NewUserSubscriber, "UserSubscriber"),

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

	// IMPORTANT! メッセージブローカーの場合は、constructorsではなく、invocationsに登録する
	dikit.AppendInvocations([]any{
		dikit.InjectSubscriber(NewSubscribeModule, "UserSubscriber"),
	})
}
