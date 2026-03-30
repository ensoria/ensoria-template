package db

import (
	"context"
	"fmt"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	workerDB "github.com/ensoria/worker/pkg/database"
)

// DatabaseClient is a common interface for database clients.
type DatabaseClient interface {
	Close() error
	Ping(ctx context.Context) error
}

// DatabaseConfig is a common interface for database configurations.
type DatabaseConfig interface {
	workerDB.DatabaseConfig | schedulerDB.DatabaseConfig
}

// ClientConstructor is a function type that creates a DatabaseClient from a config.
type ClientConstructor[C DatabaseConfig, T DatabaseClient] func(*C) (T, error)

// NewDefaultDBClient creates a generic database client factory.
func NewDefaultDBClient[C DatabaseConfig, T DatabaseClient](
	envVal *string,
	dbType string,
	newClient ClientConstructor[C, T],
	configFactory func(dbType string) *C,
) func(lc dikit.LC) (T, error) {
	return func(lc dikit.LC) (T, error) {
		cfg := configFactory(dbType)

		client, err := newClient(cfg)
		if err != nil {
			var zero T
			return zero, err
		}

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := client.Ping(ctx); err != nil {
					return fmt.Errorf("DB connection check failed (%s): %w", dbType, err)
				}
				loggear.Info("DB connection verified", "type", dbType)
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down DB connection", "type", dbType)
				return client.Close()
			},
		})

		return client, nil
	}
}

// NewDefaultWorkerDBClient creates a worker database client.
func NewDefaultWorkerDBClient(envVal *string) func(lc dikit.LC) (workerDB.DatabaseClient, error) {
	return NewDefaultDBClient(
		envVal,
		string(workerDB.DBTypePostgreSQL), // TODO: envValとconfigパッケージを使って設定を取得するようにする
		workerDB.NewDatabaseClient,
		func(dbType string) *workerDB.DatabaseConfig {
			// TODO: envValとconfigパッケージを使って設定を取得するようにする
			return &workerDB.DatabaseConfig{
				Type:     workerDB.DBType(dbType),
				Host:     "localhost",
				Port:     5432,
				User:     "ensoria",
				Password: "ensoria",
				Database: "ensoria",
				TLSMode:  "disable",
			}
		},
	)
}

// NewDefaultSchedulerDBClient creates a scheduler database client.
func NewDefaultSchedulerDBClient(envVal *string) func(lc dikit.LC) (schedulerDB.DatabaseClient, error) {
	return NewDefaultDBClient(
		envVal,
		string(schedulerDB.DBTypePostgreSQL), // TODO: envValとconfigパッケージを使って設定を取得するようにする
		schedulerDB.NewDatabaseClient,
		func(dbType string) *schedulerDB.DatabaseConfig {
			// TODO: envValとconfigパッケージを使って設定を取得するようにする
			return &schedulerDB.DatabaseConfig{
				Type:     schedulerDB.DBType(dbType),
				Host:     "localhost",
				Port:     5432,
				User:     "ensoria",
				Password: "ensoria",
				Database: "ensoria",
				TLSMode:  "disable",
			}
		},
	)
}
