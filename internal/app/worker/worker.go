package worker

import (
	"context"

	_ "github.com/ensoria/ensoria-template/internal/app/worker/api"
	appJob "github.com/ensoria/ensoria-template/internal/app/worker/job"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/worker/pkg/database"
	"github.com/ensoria/worker/pkg/history"
	"github.com/ensoria/worker/pkg/queue"
	"github.com/ensoria/worker/pkg/worker"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

// workerJobSource names the worker in the records written about the
// who-started-this value a job carries.
const workerJobSource = "worker job"

func NewWorker(
	lc dikit.LC,
	cacheClient *goredis.Client,
	dbClient database.DatabaseClient,
	jobs []*appJob.JobHandler,
	claims *initiator.ClaimSet,
) *worker.Worker {

	qStorage := queue.NewRedisQueue(cacheClient)
	historyRepo := history.NewRepository(dbClient)
	w := worker.New(
		qStorage,
		append(workerOptions(claims), worker.WithHistory(historyRepo))...,
	)

	for _, j := range jobs {
		w.Register(j.Name, j.Handler, j.Options)
	}

	// ワーカーのContext
	var workerCtx context.Context
	var workerCancel context.CancelFunc

	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			workerCtx, workerCancel = context.WithCancel(context.Background())
			go func() {
				loggear.Info("Starting worker...")
				w.Start(workerCtx)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			loggear.Info("Stopping worker...")
			workerCancel()
			return w.Shutdown(ctx)
		},
	})

	return w

}

// workerOptions are the options every worker of this application is built
// with, whatever it stores jobs in.
//
// The propagator carries who started the work (initiator.From) with each job:
// the worker writes it into the job's metadata inside Enqueue and puts it back
// on the handler's context before the job runs, so a job enqueued while serving
// a request runs as that caller's work, and one a scheduled task enqueued runs
// as the system's. The claims kept are the application's Initiator claims.
//
// ⚠ It travels in the context's values, so enqueue with the caller's context
// (context.WithoutCancel(ctx), detaching only its cancellation). A job enqueued
// with context.Background() carries nothing and runs as unknown.
func workerOptions(claims *initiator.ClaimSet) []worker.Option {
	return []worker.Option{
		worker.WithPropagator(initiator.NewPropagator(claims, workerJobSource)),
	}
}

// NewEnqueuer は *worker.Worker を worker.Enqueuer として提供する変換関数。
// サービス層など、Enqueue機能のみ必要な箇所で使用される。
func NewEnqueuer(w *worker.Worker) worker.Enqueuer {
	return w
}

func InjectWorkerJobs(f any) any {
	return fx.Annotate(
		f,
		fx.ParamTags(``, ``, ``, dikit.GroupTagWorkerJobs),
	)
}
