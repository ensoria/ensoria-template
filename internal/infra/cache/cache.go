package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cacheotter"
	"github.com/ensoria/cache/pkg/cacheredis"
	"github.com/ensoria/cache/pkg/cachetiered"
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	enclikeystore "github.com/ensoria/encli/pkg/keystore"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	goredis "github.com/redis/go-redis/v9"
)

const (
	// TODO: align with the config/module name.
	cacheKeyPrefix = "app"
	// cacheName is recorded on the tier metrics (attribute cache.name).
	cacheName = "app"
)

// The purpose names that appear in connection log lines and error messages.
// Each one has its own Redis database, so telling them apart in a log matters.
const (
	appCachePurpose  = "cache"
	workerPurpose    = "worker cache"
	schedulerPurpose = "scheduler cache"
	keyStorePurpose  = "API key store"
	sessionPurpose   = "session store"
)

// redisOptions converts a configured Redis connection into go-redis options.
//
// It is the single place the configuration's shape meets the client's, which is
// why it is a plain function rather than inline setup: the mapping is worth
// testing on its own, and every purpose has to be mapped the same way.
func redisOptions(cfg *appconfig.Redis) *goredis.Options {
	if cfg == nil {
		return nil
	}

	opts := &goredis.Options{
		Addr:     net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Username: cfg.User,
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	// A non-nil TLSConfig is what turns encryption on for go-redis, so it is
	// only set when the configuration asks for it. ServerName is left empty on
	// purpose: the client dials with tls.DialWithDialer, which fills it in from
	// the address being dialed.
	if cfg.TLSEnabled() {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return opts
}

// defaultParams reads the application-wide configuration. Every connection here
// belongs to the application rather than to one module, so they all read the
// default module's settings.
func defaultParams() (*appconfig.Parameters, error) {
	params, err := registry.ModuleParams("default")
	if err != nil {
		return nil, fmt.Errorf("cache configuration is unavailable: %w", err)
	}
	return params, nil
}

// newRedisClient builds a client for one purpose and hangs its connection check
// and shutdown off the lifecycle.
//
// The check is a hook rather than part of construction so that every connection
// the application needs is dialed at startup: a Redis that is unreachable
// should stop the application, not the first request that happens to need it.
func newRedisClient(lc dikit.LC, cfg *appconfig.Redis, purpose string) *goredis.Client {
	client := goredis.NewClient(redisOptions(cfg))

	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			if err := client.Ping(ctx).Err(); err != nil {
				return fmt.Errorf("%s connection check failed: %w", purpose, err)
			}
			loggear.Info("Redis connection verified", "purpose", purpose, "addr", cfg.Host, "db", cfg.DB)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			loggear.Info("Shutting down Redis connection", "purpose", purpose)
			return client.Close()
		},
	})

	return client
}

// NewDefaultCache builds the application cache as a cachetiered.Cache: a bounded
// in-process otter L1 over a Redis L2, exposed as enscache.Cache for DI. The L2
// Redis client is owned here (its own database, separate from the worker queue
// and the scheduler state) and closed on shutdown along with the tiered cache.
func NewDefaultCache(envVal *string) func(lc dikit.LC) (enscache.Cache, error) {
	return func(lc dikit.LC) (enscache.Cache, error) {
		params, err := defaultParams()
		if err != nil {
			return nil, err
		}
		cfg := params.Cache

		client := goredis.NewClient(redisOptions(cfg.Redis))

		// L1: bounded in-process otter store. L2: raw redis store. The codec is
		// applied once, on top, by cachetiered.New.
		l1, err := cacheotter.NewStore(cacheKeyPrefix, cacheotter.MaxEntries(cfg.Otter.MaxEntries))
		if err != nil {
			return nil, fmt.Errorf("cache L1 init failed: %w", err)
		}
		l2 := cacheredis.NewStore(client, cacheKeyPrefix)
		c, err := cachetiered.New(l1, l2,
			cachetiered.WithNearTTL(cfg.NearTTL),
			cachetiered.WithName(cacheName),
		)
		if err != nil {
			return nil, fmt.Errorf("cache init failed: %w", err)
		}

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := client.Ping(ctx).Err(); err != nil {
					return fmt.Errorf("%s connection check failed: %w", appCachePurpose, err)
				}
				loggear.Info("Redis connection verified",
					"purpose", appCachePurpose, "addr", cfg.Redis.Host, "db", cfg.Redis.DB)
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

// NewKeyStoreCache builds the store the built-in API key store reads keys from.
//
// It is a plain cacheredis rather than the tiered cache the application uses for
// its own data, and deliberately so: withdrawing a key has to take effect
// everywhere at once, and an in-process copy would keep answering with a key
// that was deleted until that copy expired.
//
// The connection is its own — its own Redis database, dialed and closed with the
// application — because the keys are neither the application's cache nor its job
// queue, and sharing a keyspace with the cache would put credentials behind an
// eviction policy.
//
// The key prefix comes from the shared format package rather than from a
// constant here, so that the records this reads are the ones encli writes.
func NewKeyStoreCache(lc dikit.LC, cfg *appconfig.Redis) enscache.Cache {
	return cacheredis.New(newRedisClient(lc, cfg, keyStorePurpose), enclikeystore.RedisNamespace)
}

// NewSessionCache builds the store browser sessions are kept in.
//
// Like the key store it is a plain cacheredis rather than the tiered cache, and
// for a stronger reason: a session read from a process-local copy outlives its
// own revocation on the node holding it. Signing out returns, the next request
// goes to that node, and the caller is still signed in — which is exactly the
// guarantee a server-side session store is chosen for.
//
// ⚠ Its Redis database is its own (DefaultSessionRedisDB). Sharing a keyspace
// with the cache would put sessions behind an eviction policy, and an evicted
// session is a signed-out user.
func NewSessionCache(lc dikit.LC, cfg *appconfig.Redis) enscache.Cache {
	return cacheredis.New(newRedisClient(lc, cfg, sessionPurpose), sessionCacheKeyPrefix)
}

// sessionCacheKeyPrefix namespaces the session records. Unlike the API key
// store's, this prefix is not shared with anything outside the application:
// nothing but the application itself reads or writes a session.
const sessionCacheKeyPrefix = "session"

// NewDefaultWorkerCacheClient builds the Redis client the job queue runs on.
func NewDefaultWorkerCacheClient(envVal *string) func(lc dikit.LC) (*goredis.Client, error) {
	return func(lc dikit.LC) (*goredis.Client, error) {
		params, err := defaultParams()
		if err != nil {
			return nil, err
		}

		return newRedisClient(lc, params.Worker.Redis, workerPurpose), nil
	}
}

// NewDefaultSchedulerCacheClient builds the Redis client the scheduler keeps its
// locks and control state in.
func NewDefaultSchedulerCacheClient(envVal *string) func(lc dikit.LC) (*goredis.Client, error) {
	return func(lc dikit.LC) (*goredis.Client, error) {
		params, err := defaultParams()
		if err != nil {
			return nil, err
		}

		return newRedisClient(lc, params.Scheduler.Redis, schedulerPurpose), nil
	}
}
