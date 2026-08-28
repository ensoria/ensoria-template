package server

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	authApp "github.com/ensoria/ensoria-template/internal/app/auth"
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
	"github.com/ensoria/ensoria-template/internal/infra/storage"
	_ "github.com/ensoria/ensoria-template/internal/module"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	_ "github.com/ensoria/ensoria-template/internal/query"
	"github.com/ensoria/loggear/pkg/loggear"
)

func Run(envVal *string) error {
	if err := registry.InitializeConfiguration(*envVal, assets.ConfigFS(*envVal), "internal", "config"); err != nil {
		return fmt.Errorf("app initialization error: %w", err)
	}

	// 宣言(Endpoint.Success / Responses)と実挙動の不一致を、開発環境では即座に失敗させる。
	// 生成ドキュメントが黙って実装から乖離しないようにするための検査。
	restkit.SetStrictDeclarations(restkit.StrictForEnv(*envVal))

	dikit.AppendConstructors([]any{
		// アプリのルートコンテキスト（常駐処理の生存期間 = アプリの生存期間）
		dikit.ProvideRootContext,

		// infra
		cache.NewDefaultWorkerCacheClient(envVal),
		cache.NewDefaultCache(envVal),
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

	params, err := registry.ModuleParams("default")
	if err != nil {
		return fmt.Errorf("app initialization error: %w", err)
	}
	// The global log level is applied here rather than from inside the graph.
	// An application that drops the HTTP app — the template exists to be
	// modified — would otherwise lose its log level without a word, and server
	// and scheduler would apply it at different points of their invocation
	// order. Settling it before fx is built also means anything logged while the
	// graph is constructed already obeys it.
	loggear.SetLogLevel(params.Log.Level)
	outputFxLog := params.Log.Level == loggear.LogLevelDebug

	return dikit.ProvideAndRun(dikit.Constructors(), dikit.Invocations(), outputFxLog)
}
