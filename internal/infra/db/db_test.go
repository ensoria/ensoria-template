package db

import (
	"time"

	"github.com/ensoria/config/pkg/appconfig"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	workerDB "github.com/ensoria/worker/pkg/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("dbSettingsFrom", func() {
	Describe("the driver mapping", func() {
		// appconfig normalizes "sqlite" to "sqlite3" while the libraries spell
		// it "sqlite", so the names cannot simply be cast across.
		DescribeTable("maps a configured driver onto the library's name",
			func(configured, expected string) {
				s, err := dbSettingsFrom(&appconfig.DB{Driver: configured})

				Expect(err).NotTo(HaveOccurred())
				Expect(s.Type).To(Equal(expected))
			},
			Entry("postgres", "postgres", string(workerDB.DBTypePostgreSQL)),
			Entry("mysql", "mysql", string(workerDB.DBTypeMySQL)),
			Entry("sqlite3", "sqlite3", string(workerDB.DBTypeSQLite)),
		)

		It("names the setting and the working drivers when one is unsupported", func() {
			_, err := dbSettingsFrom(&appconfig.DB{Driver: "cockroach"})

			Expect(err).To(MatchError(ContainSubstring("DB_DRIVER")))
			Expect(err).To(MatchError(ContainSubstring("cockroach")))
			Expect(err).To(MatchError(ContainSubstring("postgres")))
		})

		It("rejects an empty driver", func() {
			_, err := dbSettingsFrom(&appconfig.DB{})

			Expect(err).To(HaveOccurred())
		})

		It("rejects a nil configuration rather than panicking", func() {
			_, err := dbSettingsFrom(nil)

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("the connection fields", func() {
		It("carries host, port, credentials and the SSL mode", func() {
			s, err := dbSettingsFrom(&appconfig.DB{
				Driver:   "postgres",
				Host:     "history.internal",
				Port:     5432,
				User:     "app",
				Password: "secret",
				DBName:   "worker_history",
				SSLMode:  "require",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(s.Host).To(Equal("history.internal"))
			Expect(s.Port).To(Equal(5432))
			Expect(s.User).To(Equal("app"))
			Expect(s.Password).To(Equal("secret"))
			Expect(s.Database).To(Equal("worker_history"))
			// The libraries call it TLSMode; the value domain is the driver's.
			Expect(s.TLSMode).To(Equal("require"))
		})

		It("carries the pool settings", func() {
			s, err := dbSettingsFrom(&appconfig.DB{
				Driver:          "postgres",
				MaxOpenConns:    40,
				MaxIdleConns:    7,
				ConnMaxLifetime: 3 * time.Minute,
				ConnMaxIdleTime: time.Minute,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(s.MaxOpenConns).To(Equal(40))
			Expect(s.MaxIdleConns).To(Equal(7))
			Expect(s.ConnMaxLifetime).To(Equal(3 * time.Minute))
			Expect(s.ConnMaxIdleTime).To(Equal(time.Minute))
		})
	})

	// SQLite has no host: the libraries read the file path out of Database.
	Describe("where SQLite keeps its file", func() {
		It("uses DB_NAME when it is set", func() {
			s, err := dbSettingsFrom(&appconfig.DB{
				Driver: "sqlite3",
				DBName: "./worker.db",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(s.Database).To(Equal("./worker.db"))
		})

		It("falls back to DB_HOST, which is where the template configs put it", func() {
			s, err := dbSettingsFrom(&appconfig.DB{
				Driver: "sqlite3",
				Host:   "./tmp/ensoria.sqlite",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(s.Database).To(Equal("./tmp/ensoria.sqlite"))
		})

		It("prefers DB_NAME when both are set", func() {
			s, err := dbSettingsFrom(&appconfig.DB{
				Driver: "sqlite3",
				Host:   "./tmp/ensoria.sqlite",
				DBName: "./worker.db",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(s.Database).To(Equal("./worker.db"))
		})

		// A server's host is not a database name, so the fallback must not
		// leak into the other drivers.
		It("does not fall back to the host on a server driver", func() {
			s, err := dbSettingsFrom(&appconfig.DB{
				Driver: "postgres",
				Host:   "history.internal",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(s.Database).To(BeEmpty())
		})
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
		got, err := workerDBConfig(cfg())

		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(&workerDB.DatabaseConfig{
			Type:            workerDB.DBTypePostgreSQL,
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

	It("builds the scheduler library's config", func() {
		got, err := schedulerDBConfig(cfg())

		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(&schedulerDB.DatabaseConfig{
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

	It("reports an unsupported driver from both", func() {
		bad := &appconfig.DB{Driver: "cockroach"}

		_, workerErr := workerDBConfig(bad)
		_, schedulerErr := schedulerDBConfig(bad)

		Expect(workerErr).To(HaveOccurred())
		Expect(schedulerErr).To(HaveOccurred())
	})
})
