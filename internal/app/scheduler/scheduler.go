package scheduler

import (
	"context"
	"fmt"

	_ "github.com/ensoria/ensoria-template/internal/app/scheduler/api"
	"github.com/ensoria/ensoria-template/internal/app/scheduler/task"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/scheduler/pkg/control"
	"github.com/ensoria/scheduler/pkg/database"
	"github.com/ensoria/scheduler/pkg/distributed"
	"github.com/ensoria/scheduler/pkg/recovery"
	"github.com/ensoria/scheduler/pkg/scheduler"
	sched "github.com/ensoria/scheduler/pkg/scheduler"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

func NewScheduler(
	redisClient *goredis.Client,
	dbClient database.DatabaseClient,
) (*scheduler.Scheduler, error) {
	backend, err := distributed.NewBackend(&distributed.Config{
		BackendType: distributed.BackendTypeRedis,
		Client:      redisClient,
	})
	if err != nil {
		return nil, err
	}

	recoveryRepo := recovery.NewRedisStateRepository(redisClient)
	controlRepo := control.NewRedisStateRepository(redisClient, "")

	s := scheduler.New(backend,
		scheduler.WithLogger(loggear.GetLogger()),
		scheduler.WithRecovery(recoveryRepo),
		scheduler.WithControl(controlRepo),
		scheduler.WithHistory(dbClient),
	)

	return s, nil

}

func NewSchedulerApp(lc dikit.LC, s *scheduler.Scheduler, tasks []*task.ScheduledTask) error {
	RegisterTasks(s, tasks)

	// REFACTOR: RegisterSchedulerLifeCycle()関数に移動
	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			loggear.Info("Starting scheduler...")
			// OnStartのctxは起動フェーズ用なので、OnStart が完了すると（あるいはタイムアウトすると）キャンセルされる
			// スケジューラーのような長時間実行するサービスには、独立した context.Background() を渡す必要がある
			// これにより、OnStartが完了してもスケジューラーは動き続ける
			if err := s.Start(context.Background()); err != nil {
				return fmt.Errorf("failed to start scheduler: %w", err)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// シャットダウンcontextは、ctxを使う
			if err := s.Shutdown(ctx); err != nil {
				return fmt.Errorf("scheduler shutdown error: %v", err)
			}
			return nil
		},
	})

	return nil
}

func RegisterTasks(s *sched.Scheduler, tasks []*task.ScheduledTask) error {
	for _, task := range tasks {
		if err := s.SetSchedule(task.Name, task.Cron, task.Task); err != nil {
			return err
		}
	}

	return nil
}

func InjectScheduledTasks(f any) any {
	return fx.Annotate(
		f,
		fx.ParamTags(``, ``, dikit.GroupTagScheduledTasks),
	)
}
