package scheduler

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	"github.com/ensoria/ensoria-template/internal/infra/cache"
	"github.com/ensoria/ensoria-template/internal/infra/db"
	"github.com/ensoria/ensoria-template/internal/infra/keystore"
	"github.com/ensoria/ensoria-template/internal/infra/mb"
	"github.com/ensoria/ensoria-template/internal/infra/session"
	"github.com/ensoria/ensoria-template/internal/infra/storage"
	"github.com/ensoria/websocket/pkg/wsrouter"

	authApp "github.com/ensoria/ensoria-template/internal/app/auth"
	"github.com/ensoria/ensoria-template/internal/app/bootstrap"
	httpApp "github.com/ensoria/ensoria-template/internal/app/http"
	mbApp "github.com/ensoria/ensoria-template/internal/app/mb"
	schedulerApp "github.com/ensoria/ensoria-template/internal/app/scheduler"
	workerApp "github.com/ensoria/ensoria-template/internal/app/worker"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	_ "github.com/ensoria/ensoria-template/internal/module"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	_ "github.com/ensoria/ensoria-template/internal/query"
)

func Start(envVal *string) error {
	if err := registry.InitializeConfiguration(*envVal, assets.ConfigFS(*envVal), "internal", "config"); err != nil {
		return fmt.Errorf("app initialization error: %w", err)
	}

	// The global settings — log level, strict declaration mode — are applied
	// here so that server and scheduler cannot drift apart on them. The
	// scheduler serves HTTP endpoints of its own, so strict mode is as
	// load-bearing here as it is in the server.
	outputFxLog, err := bootstrap.ApplyGlobalSettings(envVal, registry.DefaultRegistry())
	if err != nil {
		return err
	}

	dikit.AppendConstructors([]any{
		// アプリのルートコンテキスト（常駐処理の生存期間 = アプリの生存期間）
		// mbApp.NewSubscribe が dikit.RootContext に依存するため、購読を行うこのappでも必須
		dikit.ProvideRootContext,

		// The configuration this application resolved above. Most code reads it
		// through the registry package's own functions, which answer from this
		// same instance; it is put in the graph for the constructors that take
		// it as an argument so their tests can hand them another one.
		registry.DefaultRegistry,

		// infra
		// workerとinjectするインスタンスを分けるため、タグ名を付ける
		dikit.ProvideNamed(cache.NewDefaultSchedulerCacheClient(envVal), "schedulerCache"),
		// アプリ用キャッシュ（enscache.Cache）。自前のL2クライアントを持つため named 不要
		cache.NewDefaultCache(envVal),
		db.NewDefaultSchedulerDBClient(envVal),
		keystore.NewAPIKeyStore(envVal),
		session.NewSessionStore(envVal),

		// TODO: 無くてもいいようにする?
		dikit.ProvideNamed(cache.NewDefaultWorkerCacheClient(envVal), "workerCache"),
		db.NewDefaultWorkerDBClient(envVal),
		mb.NewSubscriberConnection(envVal),
		mb.NewPublisherConnection(envVal),
		storage.NewDefaultStorage(envVal),
		storage.NewDefaultFileSystem,
		mbApp.NewSubscribe,
		mbApp.NewPublish,
		dikit.InjectWithTags(workerApp.NewWorker, ``, `name:"workerCache"`, ``, `group:"worker_jobs"`),
		workerApp.NewEnqueuer,

		// scheduler
		// タグ名の付いたキャッシュクライアントを注入
		dikit.InjectWithTags(schedulerApp.NewScheduler, `name:"schedulerCache"`, ``),

		// FIXME: schedulerだけでなく、moduleのものも全部うごいてしまっているので修正
		// scheduler管理用のエンドポイントのみ
		authApp.NewVerifier(envVal),
		// Which origins are this deployment's own frontend, resolved once and read
		// by CORS, the cross-origin check and the WebSocket upgrade guard.
		wsApp.NewTrustedOrigins,
		httpApp.InjectHTTPModules(httpApp.CreateHTTPPipeline(envVal)),
		NewEmptyWSRouter,
	})

	dikit.AppendInvocations([]any{
		schedulerApp.InjectScheduledTasks(schedulerApp.NewSchedulerApp),
		httpApp.NewHTTPApp(envVal),
	})

	return dikit.ProvideAndRun(dikit.Constructors(), dikit.Invocations(), outputFxLog)
}

// schedulerではwsrouterは使わないが、HTTPパイプラインの初期化で必要になるため、空のrouterを提供する
func NewEmptyWSRouter() *wsrouter.Router {
	return &wsrouter.Router{}
}
