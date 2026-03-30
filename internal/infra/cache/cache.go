package cache

import (
	"context"
	"fmt"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	goredis "github.com/redis/go-redis/v9"
)

func NewDefaultWorkerCacheClient(envVal *string) func(lc dikit.LC) *goredis.Client {
	return func(lc dikit.LC) *goredis.Client {
		// TODO: envValとconfigパッケージを使って設定を取得するようにする
		// params := registry.ModuleParams("default")
		client := goredis.NewClient(&goredis.Options{
			Addr: "localhost:6379",
			DB:   0,
		})

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := client.Ping(ctx).Err(); err != nil {
					return fmt.Errorf("worker cache connection check failed: %w", err)
				}
				loggear.Info("Worker cache connection verified")
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down worker cache")
				return client.Close()
			},
		})

		return client
	}

}

func NewDefaultSchedulerCacheClient(envVal *string) func(lc dikit.LC) *goredis.Client {
	return func(lc dikit.LC) *goredis.Client {
		// TODO: envValとconfigパッケージを使って設定を取得するようにする
		// params := registry.ModuleParams("default")
		client := goredis.NewClient(&goredis.Options{
			Addr: "localhost:6379",
			DB:   1,
		})

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := client.Ping(ctx).Err(); err != nil {
					return fmt.Errorf("scheduler cache connection check failed: %w", err)
				}
				loggear.Info("Scheduler cache connection verified")
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down scheduler cache")
				return client.Close()
			},
		})

		return client
	}

}
