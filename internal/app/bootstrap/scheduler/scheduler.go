package scheduler

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/infra/cache"
	"github.com/ensoria/ensoria-template/internal/infra/db"
	"github.com/ensoria/ensoria-template/internal/infra/mb"
	"github.com/ensoria/websocket/pkg/wsrouter"

	httpApp "github.com/ensoria/ensoria-template/internal/app/http"
	mbApp "github.com/ensoria/ensoria-template/internal/app/mb"
	schedulerApp "github.com/ensoria/ensoria-template/internal/app/scheduler"
	workerApp "github.com/ensoria/ensoria-template/internal/app/worker"
	_ "github.com/ensoria/ensoria-template/internal/module"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
)

func Start(envVal *string) error {
	registry.InitializeConfiguration(envVal, "./internal", "config")

	dikit.AppendConstructors([]any{
		// infra
		// workerとinjectするインスタンスを分けるため、タグ名を付ける
		dikit.ProvideNamed(cache.NewDefaultSchedulerCacheClient(envVal), "schedulerCache"),
		db.NewDefaultSchedulerDBClient(envVal),

		// TODO: 無くてもいいようにする?
		dikit.ProvideNamed(cache.NewDefaultWorkerCacheClient(envVal), "workerCache"),
		db.NewDefaultWorkerDBClient(envVal),
		mb.NewSubscriberConnection(envVal),
		mb.NewPublisherConnection(envVal),
		mbApp.NewSubscribe,
		mbApp.NewPublish,
		dikit.InjectWithTags(workerApp.NewWorker, ``, `name:"workerCache"`, ``, `group:"worker_jobs"`),
		workerApp.NewEnqueuer,

		// scheduler
		// タグ名の付いたキャッシュクライアントを注入
		dikit.InjectWithTags(schedulerApp.NewScheduler, `name:"schedulerCache"`, ``),

		// scheduler管理用のエンドポイントのみ
		httpApp.InjectHTTPModules(httpApp.CreateHTTPPipeline),
		NewEmptyWSRouter,
	})

	dikit.AppendInvocations([]any{
		schedulerApp.InjectScheduledTasks(schedulerApp.NewSchedulerApp),
		httpApp.NewHTTPApp(envVal),
	})

	params, err := registry.ModuleParams("default")
	if err != nil {
		return fmt.Errorf("app initialization error: %w", err)
	}
	outputFxLog := params.Log.Level == "debug"

	dikit.ProvideAndRun(dikit.Constructors(), dikit.Invocations(), outputFxLog)

	return nil
}

// schedulerではwsrouterは使わないが、HTTPパイプラインの初期化で必要になるため、空のrouterを提供する
func NewEmptyWSRouter() *wsrouter.Router {
	return &wsrouter.Router{}
}
