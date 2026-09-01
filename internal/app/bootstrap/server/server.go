package server

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	authApp "github.com/ensoria/ensoria-template/internal/app/auth"
	"github.com/ensoria/ensoria-template/internal/app/bootstrap"
	grpcApp "github.com/ensoria/ensoria-template/internal/app/grpc"
	httpApp "github.com/ensoria/ensoria-template/internal/app/http"
	mbApp "github.com/ensoria/ensoria-template/internal/app/mb"
	workerApp "github.com/ensoria/ensoria-template/internal/app/worker"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/infra/cache"
	"github.com/ensoria/ensoria-template/internal/infra/db"
	_ "github.com/ensoria/ensoria-template/internal/infra/grpcclt"
	"github.com/ensoria/ensoria-template/internal/infra/keystore"
	"github.com/ensoria/ensoria-template/internal/infra/mb"
	_ "github.com/ensoria/ensoria-template/internal/infra/mb"
	"github.com/ensoria/ensoria-template/internal/infra/storage"
	_ "github.com/ensoria/ensoria-template/internal/module"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	_ "github.com/ensoria/ensoria-template/internal/query"
)

func Run(envVal *string) error {
	if err := registry.InitializeConfiguration(*envVal, assets.ConfigFS(*envVal), "internal", "config"); err != nil {
		return fmt.Errorf("app initialization error: %w", err)
	}

	// The global settings — log level, strict declaration mode — are applied
	// here so that server and scheduler cannot drift apart on them.
	outputFxLog, err := bootstrap.ApplyGlobalSettings(envVal, registry.DefaultRegistry())
	if err != nil {
		return err
	}

	dikit.AppendConstructors([]any{
		// アプリのルートコンテキスト（常駐処理の生存期間 = アプリの生存期間）
		dikit.ProvideRootContext,

		// The configuration this application resolved above. Most code reads it
		// through the registry package's own functions, which answer from this
		// same instance; it is put in the graph for the constructors that take
		// it as an argument so their tests can hand them another one.
		registry.DefaultRegistry,

		// infra
		cache.NewDefaultWorkerCacheClient(envVal),
		cache.NewDefaultCache(envVal),
		keystore.NewAPIKeyStore(envVal),
		db.NewDefaultWorkerDBClient(envVal),
		mb.NewSubscriberConnection(envVal),
		mb.NewPublisherConnection(envVal),
		storage.NewDefaultStorage(envVal),
		storage.NewDefaultFileSystem,

		// controllers
		authApp.NewVerifier(envVal),
		httpApp.InjectHTTPModules(httpApp.CreateHTTPPipeline),
		wsApp.InjectWSModules(wsApp.CreateWSRouter),
		mbApp.NewSubscribe,
		mbApp.NewPublish,

		// worker
		workerApp.InjectWorkerJobs(workerApp.NewWorker),
		workerApp.NewEnqueuer,
	})

	dikit.AppendInvocations([]any{
		// application invocations
		httpApp.NewHTTPApp(envVal),
		grpcApp.InjectGRPCServices(grpcApp.NewGRPCApp(envVal)),
		// 宣言された購読をまとめて開始する。購読は戻り値を持たず誰からも参照されないため、
		// constructorsではなくinvocationsに登録する必要がある。
		mbApp.InjectMBSubscriptions(mbApp.StartSubscriptions),
	})

	return dikit.ProvideAndRun(dikit.Constructors(), dikit.Invocations(), outputFxLog)
}
