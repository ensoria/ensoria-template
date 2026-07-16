package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cacheotter"
	"github.com/ensoria/cache/pkg/cacheredis"
	"github.com/ensoria/cache/pkg/cachetiered"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	goredis "github.com/redis/go-redis/v9"
)

// Application cache configuration. Values are hardcoded for now, mirroring the
// worker/scheduler clients above.
const (
	// TODO: derive from envVal/config package instead of hardcoding.
	cacheRedisAddr = "localhost:6379"
	// cacheRedisDB isolates the application cache from the worker job queue
	// (DB 0) and the scheduler state store (DB 1). Those must never evict keys,
	// whereas a cache wants an LRU/LFU eviction policy; since maxmemory-policy is
	// instance-wide, a dedicated DB keeps their keyspaces and FLUSHDB blast radius
	// apart. For production, a separate Redis instance is recommended so the cache
	// can run its own eviction policy and memory cap.
	cacheRedisDB = 2
	// TODO: align with the config/module name.
	cacheKeyPrefix = "app"
	// l1MaxEntries bounds the in-process otter L1 by entry count.
	l1MaxEntries = 1_000
	// cacheNearTTL caps how long a value lives in L1, bounding cross-replica
	// staleness (equals cachetiered's default, set explicitly for clarity).
	cacheNearTTL = 5 * time.Second
	// cacheName is recorded on the tier metrics (attribute cache.name).
	cacheName = "app"
)

// NewDefaultCache builds the application cache as a cachetiered.Cache: a bounded
// in-process otter L1 over a Redis L2, exposed as enscache.Cache for DI. The L2
// Redis client is owned here (its own DB, separate from the worker queue and
// scheduler state) and closed on shutdown along with the tiered cache.
func NewDefaultCache(envVal *string) func(lc dikit.LC) (enscache.Cache, error) {
	return func(lc dikit.LC) (enscache.Cache, error) {
		// TODO: envValとconfigパッケージを使って設定を取得するようにする
		// worker, schedulerとは別の値になるので、設定値を分ける必要がある
		client := goredis.NewClient(&goredis.Options{
			Addr: cacheRedisAddr,
			DB:   cacheRedisDB,
		})

		// TODO: otterの設定値も、config/moduleから取得するようにする
		// L1: bounded in-process otter store. L2: raw redis store. The codec is
		// applied once, on top, by cachetiered.New.
		l1, err := cacheotter.NewStore(cacheKeyPrefix, cacheotter.MaxEntries(l1MaxEntries))
		if err != nil {
			return nil, fmt.Errorf("cache L1 init failed: %w", err)
		}
		// TODO: 設定値をconfig/moduleから取得するようにする
		l2 := cacheredis.NewStore(client, cacheKeyPrefix)
		c, err := cachetiered.New(l1, l2,
			cachetiered.WithNearTTL(cacheNearTTL),
			cachetiered.WithName(cacheName),
		)
		if err != nil {
			return nil, fmt.Errorf("cache init failed: %w", err)
		}

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := client.Ping(ctx).Err(); err != nil {
					return fmt.Errorf("cache connection check failed: %w", err)
				}
				loggear.Info("Cache connection verified")
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down cache")
				// Close the tiered cache first to stop L1 (otter) background
				// goroutines, then close the L2 Redis client owned here.
				var closeErr error
				if closer, ok := c.(enscache.Closer); ok {
					closeErr = closer.Close()
				}
				return errors.Join(closeErr, client.Close())
			},
		})

		return c, nil
	}
}

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
