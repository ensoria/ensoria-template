package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	workerDB "github.com/ensoria/worker/pkg/database"

	// The drivers register themselves under the names the configuration uses,
	// so a configured DB_DRIVER reaches sql.Open unchanged. modernc's SQLite is
	// pure Go, so a build needs no C toolchain.
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// The purpose names that appear in connection log lines and error messages.
// Both databases usually point at the same server, so telling them apart in a
// log is what makes a failure readable.
const (
	workerDBPurpose    = "worker history"
	schedulerDBPurpose = "scheduler history"
	keyStoreDBPurpose  = "API key store"
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

// The driver names the configuration resolves to, which are also the names each
// driver registers itself under, so a configured value reaches sql.Open
// unchanged. appconfig normalizes "sqlite3" to "sqlite" before it gets here.
const (
	postgresDriver = "postgres"
	mysqlDriver    = "mysql"
)

// mysqlParams is what a MySQL connection needs beyond the address.
//
// parseTime turns DATETIME and TIMESTAMP columns into time.Time instead of
// []byte, which is what lets a nullable expiry be scanned at all. loc pins the
// session to UTC so that a stored deadline means the same thing here as it did
// where it was written — the alternative is a key that expires an offset early
// or late depending on where the writer was.
const mysqlParams = "parseTime=true&loc=UTC"

// NewKeyStoreDB opens the connection the built-in API key store reads from.
//
// This is the application's own SQL connection, as opposed to the two below,
// which belong to the worker and scheduler libraries and are built by them. The
// key store's database is configured by AUTH_KEYSTORE_DB_*, falling back to the
// resolved DB_* values, so an application keeping its keys alongside the rest of
// its data configures nothing but the selector.
//
// The table it reads is created by `encli auth keystore init`; it is not part of
// the application's own schema and no migration carries it.
func NewKeyStoreDB(lc dikit.LC, cfg *appconfig.DB) (*sql.DB, error) {
	dsn, err := dataSourceName(cfg)
	if err != nil {
		return nil, err
	}

	// sql.Open validates the arguments and no more; nothing is dialed until the
	// lifecycle hook below asks for it.
	conn, err := sql.Open(cfg.Driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s DB connection could not be opened: %w", keyStoreDBPurpose, err)
	}
	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	conn.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			if err := conn.PingContext(ctx); err != nil {
				return fmt.Errorf("%s DB connection check failed: %w", keyStoreDBPurpose, err)
			}
			loggear.Info("DB connection verified",
				"purpose", keyStoreDBPurpose, "driver", cfg.Driver,
				"host", cfg.Host, "database", databaseFor(cfg))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			loggear.Info("Shutting down DB connection", "purpose", keyStoreDBPurpose)
			return conn.Close()
		},
	})

	return conn, nil
}

// dataSourceName renders a configured database as the string its driver expects.
//
// ⚠ SSLMode is passed through rather than translated. Its vocabulary is the
// driver's own (see appconfig.DB.SSLMode), and the two are deliberately not
// unified — a translation table would have a hole in exactly the modes that
// matter most.
func dataSourceName(cfg *appconfig.DB) (string, error) {
	switch cfg.Driver {
	case postgresDriver:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode), nil
	case mysqlDriver:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s&tls=%s",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, mysqlParams, cfg.SSLMode), nil
	case sqliteDriver:
		// SQLite has no server: the "database" is a file, which the
		// configuration may name under either key (see databaseFor).
		return databaseFor(cfg), nil
	default:
		return "", fmt.Errorf("cannot open a %q database: expected %q, %q or %q",
			cfg.Driver, postgresDriver, mysqlDriver, sqliteDriver)
	}
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
