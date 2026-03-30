package server

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	grpcApp "github.com/ensoria/ensoria-template/internal/app/grpc"
	httpApp "github.com/ensoria/ensoria-template/internal/app/http"
	mbApp "github.com/ensoria/ensoria-template/internal/app/mb"
	workerApp "github.com/ensoria/ensoria-template/internal/app/worker"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/infra/cache"
	"github.com/ensoria/ensoria-template/internal/infra/db"
	_ "github.com/ensoria/ensoria-template/internal/infra/grpcclt"
	"github.com/ensoria/ensoria-template/internal/infra/mb"
	_ "github.com/ensoria/ensoria-template/internal/infra/mb"
	_ "github.com/ensoria/ensoria-template/internal/module"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
)

func Run(envVal *string) error {
	registry.InitializeConfiguration(envVal, "./internal", "config")

	dikit.AppendConstructors([]any{
		// infra
		cache.NewDefaultWorkerCacheClient(envVal),
		db.NewDefaultWorkerDBClient(envVal),
		mb.NewSubscriberConnection(envVal),
		mb.NewPublisherConnection(envVal),

		// controllers
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
	})

	params, err := registry.ModuleParams("default")
	if err != nil {
		return fmt.Errorf("app initialization error: %w", err)
	}
	outputFxLog := params.Log.Level == "debug"

	dikit.ProvideAndRun(dikit.Constructors(), dikit.Invocations(), outputFxLog)

	return nil
}
