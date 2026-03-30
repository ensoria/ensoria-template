package worker

import (
	"context"

	_ "github.com/ensoria/ensoria-template/internal/app/worker/api"
	appJob "github.com/ensoria/ensoria-template/internal/app/worker/job"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/worker/pkg/database"
	"github.com/ensoria/worker/pkg/history"
	"github.com/ensoria/worker/pkg/queue"
	"github.com/ensoria/worker/pkg/worker"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

func NewWorker(
	lc dikit.LC,
	cacheClient *goredis.Client,
	dbClient database.DatabaseClient,
	jobs []*appJob.JobHandler,
) *worker.Worker {

	qStorage := queue.NewRedisQueue(cacheClient)
	historyRepo := history.NewRepository(dbClient)
	w := worker.New(
		qStorage,
		worker.WithHistory(historyRepo),
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
