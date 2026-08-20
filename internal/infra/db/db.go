package db

import (
	"context"
	"fmt"
	"time"

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

// The driver names appconfig produces. They are not the names the worker and
// scheduler libraries use, which is why dbSettingsFrom maps rather than casts:
// appconfig normalizes "sqlite" to "sqlite3", and those libraries spell the
// same driver "sqlite".
const (
	postgresDriver = "postgres"
	mysqlDriver    = "mysql"
	sqliteDriver   = "sqlite3"
)

// DatabaseClient is a common interface for database clients.
type DatabaseClient interface {
	Close() error
	Ping(ctx context.Context) error
}

// dbSettings is an appconfig.DB mapped onto the shape the worker and scheduler
// libraries share.
//
// The two libraries declare their own DatabaseConfig types with identical
// fields, so the mapping is written once here and copied into each of them by a
// trivial function. Everything that can be decided or fail — the driver name,
// an unsupported driver, where SQLite keeps its file — is decided here, which
// is what makes it worth testing on its own.
type dbSettings struct {
	Type            string
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	TLSMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// dbSettingsFrom maps a configured database onto the libraries' shape.
//
// An unsupported driver is an error rather than a pass-through: the libraries
// would reject it when they open the connection, and a message naming the
// setting and the drivers that do work is more use than one naming the type
// they could not switch on.
func dbSettingsFrom(cfg *appconfig.DB) (*dbSettings, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no database is configured")
	}

	var dbType string
	switch cfg.Driver {
	case postgresDriver:
		dbType = string(workerDB.DBTypePostgreSQL)
	case mysqlDriver:
		dbType = string(workerDB.DBTypeMySQL)
	case sqliteDriver:
		dbType = string(workerDB.DBTypeSQLite)
	default:
		return nil, fmt.Errorf(
			"unsupported DB_DRIVER %q for the worker and scheduler history databases: use %q, %q or %q",
			cfg.Driver, postgresDriver, mysqlDriver, sqliteDriver)
	}

	return &dbSettings{
		Type:            dbType,
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
	}, nil
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

// workerDBConfig builds the worker library's config from a configured database.
func workerDBConfig(cfg *appconfig.DB) (*workerDB.DatabaseConfig, error) {
	s, err := dbSettingsFrom(cfg)
	if err != nil {
		return nil, err
	}

	return &workerDB.DatabaseConfig{
		Type:            workerDB.DBType(s.Type),
		Host:            s.Host,
		Port:            s.Port,
		User:            s.User,
		Password:        s.Password,
		Database:        s.Database,
		TLSMode:         s.TLSMode,
		MaxOpenConns:    s.MaxOpenConns,
		MaxIdleConns:    s.MaxIdleConns,
		ConnMaxLifetime: s.ConnMaxLifetime,
		ConnMaxIdleTime: s.ConnMaxIdleTime,
	}, nil
}

// schedulerDBConfig builds the scheduler library's config from a configured database.
func schedulerDBConfig(cfg *appconfig.DB) (*schedulerDB.DatabaseConfig, error) {
	s, err := dbSettingsFrom(cfg)
	if err != nil {
		return nil, err
	}

	return &schedulerDB.DatabaseConfig{
		Type:            schedulerDB.DBType(s.Type),
		Host:            s.Host,
		Port:            s.Port,
		User:            s.User,
		Password:        s.Password,
		Database:        s.Database,
		TLSMode:         s.TLSMode,
		MaxOpenConns:    s.MaxOpenConns,
		MaxIdleConns:    s.MaxIdleConns,
		ConnMaxLifetime: s.ConnMaxLifetime,
		ConnMaxIdleTime: s.ConnMaxIdleTime,
	}, nil
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

		cfg, err := workerDBConfig(params.Worker.HistoryDB)
		if err != nil {
			return nil, fmt.Errorf("%s DB: %w", workerDBPurpose, err)
		}

		client, err := workerDB.NewDatabaseClient(cfg)
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

		cfg, err := schedulerDBConfig(params.Scheduler.HistoryDB)
		if err != nil {
			return nil, fmt.Errorf("%s DB: %w", schedulerDBPurpose, err)
		}

		client, err := schedulerDB.NewDatabaseClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("%s DB connection failed: %w", schedulerDBPurpose, err)
		}
		appendDBHooks(lc, client, schedulerDBPurpose, params.Scheduler.HistoryDB)

		return client, nil
	}
}
