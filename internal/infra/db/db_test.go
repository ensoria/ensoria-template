package db

import (
	"time"

	"github.com/ensoria/config/pkg/appconfig"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	workerDB "github.com/ensoria/worker/pkg/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The driver name is handed to the libraries unchanged, which is only safe
// while all three packages spell these drivers the same way. That agreement is
// what these specs hold in place: if appconfig ever normalizes to a different
// name, or a library renames one of its DBType constants, the mapping starts
// producing a driver nobody registers, and the failure would otherwise surface
// as a connection error at startup rather than here.
var _ = Describe("the shared driver vocabulary", func() {
	DescribeTable("a configured driver reaches both libraries unchanged",
		func(configured string, worker workerDB.DBType, scheduler schedulerDB.DBType) {
			params, err := (&appconfig.Parameters{}).OverwriteParams(map[string]string{
				"DB_DRIVER": configured,
				"DB_HOST":   "history.internal",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(workerDBConfig(params.DB).Type).To(Equal(worker))
			Expect(schedulerDBConfig(params.DB).Type).To(Equal(scheduler))
		},
		Entry("postgres", "postgres", workerDB.DBTypePostgreSQL, schedulerDB.DBTypePostgreSQL),
		Entry("mysql", "mysql", workerDB.DBTypeMySQL, schedulerDB.DBTypeMySQL),
		Entry("sqlite", "sqlite", workerDB.DBTypeSQLite, schedulerDB.DBTypeSQLite),
		// appconfig accepts the driver's own registered name as an alias and
		// normalizes it, so this has to arrive as "sqlite" too.
		Entry("sqlite3", "sqlite3", workerDB.DBTypeSQLite, schedulerDB.DBTypeSQLite),
	)

	It("agrees on the name this file has to recognize", func() {
		Expect(string(workerDB.DBTypeSQLite)).To(Equal(sqliteDriver))
		Expect(string(schedulerDB.DBTypeSQLite)).To(Equal(sqliteDriver))
	})

	// Deciding what an unfamiliar driver means is the libraries' job now: they
	// reject it when they open the connection, and their message names the
	// values that would have worked.
	It("passes an unrecognized driver through for the libraries to reject", func() {
		got := workerDBConfig(&appconfig.DB{Driver: "cockroach"})

		Expect(got.Type).To(Equal(workerDB.DBType("cockroach")))
	})
})

var _ = Describe("the library-specific configs", func() {
	cfg := func() *appconfig.DB {
		return &appconfig.DB{
			Driver:          "postgres",
			Host:            "history.internal",
			Port:            5432,
			User:            "app",
			Password:        "secret",
			DBName:          "history",
			SSLMode:         "require",
			MaxOpenConns:    40,
			MaxIdleConns:    7,
			ConnMaxLifetime: 3 * time.Minute,
			ConnMaxIdleTime: time.Minute,
		}
	}

	It("builds the worker library's config", func() {
		Expect(workerDBConfig(cfg())).To(Equal(&workerDB.DatabaseConfig{
			Type:     workerDB.DBTypePostgreSQL,
			Host:     "history.internal",
			Port:     5432,
			User:     "app",
			Password: "secret",
			Database: "history",
			// The libraries call it TLSMode; the value domain is the driver's.
			TLSMode:         "require",
			MaxOpenConns:    40,
			MaxIdleConns:    7,
			ConnMaxLifetime: 3 * time.Minute,
			ConnMaxIdleTime: time.Minute,
		}))
	})

	It("builds the scheduler library's config", func() {
		Expect(schedulerDBConfig(cfg())).To(Equal(&schedulerDB.DatabaseConfig{
			Type:            schedulerDB.DBTypePostgreSQL,
			Host:            "history.internal",
			Port:            5432,
			User:            "app",
			Password:        "secret",
			Database:        "history",
			TLSMode:         "require",
			MaxOpenConns:    40,
			MaxIdleConns:    7,
			ConnMaxLifetime: 3 * time.Minute,
			ConnMaxIdleTime: time.Minute,
		}))
	})
})

// SQLite has no host: the libraries read the file path out of Database.
var _ = Describe("where SQLite keeps its file", func() {
	It("uses DB_NAME when it is set", func() {
		Expect(databaseFor(&appconfig.DB{
			Driver: sqliteDriver,
			DBName: "./worker.db",
		})).To(Equal("./worker.db"))
	})

	It("falls back to DB_HOST, which is where the template configs put it", func() {
		Expect(databaseFor(&appconfig.DB{
			Driver: sqliteDriver,
			Host:   "./tmp/ensoria.sqlite",
		})).To(Equal("./tmp/ensoria.sqlite"))
	})

	It("prefers DB_NAME when both are set", func() {
		Expect(databaseFor(&appconfig.DB{
			Driver: sqliteDriver,
			Host:   "./tmp/ensoria.sqlite",
			DBName: "./worker.db",
		})).To(Equal("./worker.db"))
	})

	// A server's host is not a database name, so the fallback must not leak
	// into the other drivers.
	It("does not fall back to the host on a server driver", func() {
		Expect(databaseFor(&appconfig.DB{
			Driver: "postgres",
			Host:   "history.internal",
		})).To(BeEmpty())
	})

	// The alias is normalized before it ever reaches here, so the rule keys off
	// the normalized name alone.
	It("reaches the file path through the normalized name", func() {
		params, err := (&appconfig.Parameters{}).OverwriteParams(map[string]string{
			"DB_DRIVER": "sqlite3",
			"DB_HOST":   "./tmp/ensoria.sqlite",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(workerDBConfig(params.Worker.HistoryDB).Database).To(Equal("./tmp/ensoria.sqlite"))
	})
})
