package db

import (
	"context"
	"fmt"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	workerDB "github.com/ensoria/worker/pkg/database"
)

// The purpose names that appear in connection log lines and error messages.
// Both databases usually point at the same server, so telling them apart in a
// log is what makes a failure readable.
const (
	workerDBPurpose    = "worker history"
	schedulerDBPurpose = "scheduler history"
)

// sqliteDriver is the name appconfig normalizes both "sqlite" and "sqlite3" to.
// It is only needed to spot the one driver whose "database" is a file path.
const sqliteDriver = "sqlite"

// DatabaseClient is a common interface for database clients.
type DatabaseClient interface {
	Close() error
	Ping(ctx context.Context) error
}

// The worker and scheduler libraries declare their own DatabaseConfig types
// with identical fields, so each of the two builders below is the same mapping
// written against a different package.
//
// The driver name crosses over unchanged. appconfig, worker and scheduler all
// spell these drivers the same way — "postgres", "mysql", "sqlite" — so there
// is nothing to translate, and nothing here has to decide what an unfamiliar
// name means: the libraries reject one when they open the connection, and their
// message names the values that would have worked.

// workerDBConfig builds the worker library's config from a configured database.
func workerDBConfig(cfg *appconfig.DB) *workerDB.DatabaseConfig {
	return &workerDB.DatabaseConfig{
		Type:            workerDB.DBType(cfg.Driver),
		Host:            cfg.Host,
		Port:            cfg.Port,
		User:            cfg.User,
		Password:        cfg.Password,
		Database:        databaseFor(cfg),
		TLSMode:         cfg.SSLMode,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	}
}

// schedulerDBConfig builds the scheduler library's config from a configured database.
func schedulerDBConfig(cfg *appconfig.DB) *schedulerDB.DatabaseConfig {
	return &schedulerDB.DatabaseConfig{
		Type:            schedulerDB.DBType(cfg.Driver),
		Host:            cfg.Host,
		Port:            cfg.Port,
		User:            cfg.User,
		Password:        cfg.Password,
		Database:        databaseFor(cfg),
		TLSMode:         cfg.SSLMode,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	}
}

// databaseFor resolves what the libraries call Database.
//
// For a server it is the database name. For SQLite it is a file path, and the
// configuration can spell that in either of two places: DB_NAME says what the
// database is called, which for SQLite is the file, while DB_HOST is where the
// existing template configs put it. DB_NAME wins when both are set, because it
// is the field that means "which database"; SQLite has no host to speak of.
func databaseFor(cfg *appconfig.DB) string {
	if cfg.Driver == sqliteDriver && cfg.DBName == "" {
		return cfg.Host
	}
	return cfg.DBName
}

// defaultParams reads the application-wide configuration. Both history
// databases belong to the application rather than to one module.
func defaultParams() (*appconfig.Parameters, error) {
	params, err := registry.ModuleParams("default")
	if err != nil {
		return nil, fmt.Errorf("database configuration is unavailable: %w", err)
	}
	return params, nil
}

// appendDBHooks hangs a client's connection check and shutdown off the lifecycle.
//
// The check is a hook rather than part of construction so that every connection
// the application needs is dialed at startup: an unreachable database should
// stop the application, not the first job that happens to need it.
func appendDBHooks(lc dikit.LC, client DatabaseClient, purpose string, cfg *appconfig.DB) {
	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			if err := client.Ping(ctx); err != nil {
				return fmt.Errorf("%s DB connection check failed: %w", purpose, err)
			}
			loggear.Info("DB connection verified",
				"purpose", purpose, "driver", cfg.Driver, "host", cfg.Host, "database", cfg.DBName)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			loggear.Info("Shutting down DB connection", "purpose", purpose)
			return client.Close()
		},
	})
}

// NewDefaultWorkerDBClient creates the client the worker writes job history with.
func NewDefaultWorkerDBClient(envVal *string) func(lc dikit.LC) (workerDB.DatabaseClient, error) {
	return func(lc dikit.LC) (workerDB.DatabaseClient, error) {
		params, err := defaultParams()
		if err != nil {
			return nil, err
		}

		if params.Worker == nil || params.Worker.HistoryDB == nil {
			return nil, fmt.Errorf("%s DB: no database is configured", workerDBPurpose)
		}

		client, err := workerDB.NewDatabaseClient(workerDBConfig(params.Worker.HistoryDB))
		if err != nil {
			return nil, fmt.Errorf("%s DB connection failed: %w", workerDBPurpose, err)
		}
		appendDBHooks(lc, client, workerDBPurpose, params.Worker.HistoryDB)

		return client, nil
	}
}

// NewDefaultSchedulerDBClient creates the client the scheduler writes run history with.
func NewDefaultSchedulerDBClient(envVal *string) func(lc dikit.LC) (schedulerDB.DatabaseClient, error) {
	return func(lc dikit.LC) (schedulerDB.DatabaseClient, error) {
		params, err := defaultParams()
		if err != nil {
			return nil, err
		}

		if params.Scheduler == nil || params.Scheduler.HistoryDB == nil {
			return nil, fmt.Errorf("%s DB: no database is configured", schedulerDBPurpose)
		}

		client, err := schedulerDB.NewDatabaseClient(schedulerDBConfig(params.Scheduler.HistoryDB))
		if err != nil {
			return nil, fmt.Errorf("%s DB connection failed: %w", schedulerDBPurpose, err)
		}
		appendDBHooks(lc, client, schedulerDBPurpose, params.Scheduler.HistoryDB)

		return client, nil
	}
}
