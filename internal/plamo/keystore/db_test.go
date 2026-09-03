package keystore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	enclikeystore "github.com/ensoria/encli/pkg/keystore"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/keystore"
	_ "modernc.org/sqlite"
)

// createTable is the table `encli auth keystore init` creates, taken from the
// same place the command takes it.
//
// It used to be a copy written out here, which pinned nothing: a rename in encli
// left this copy and the query agreeing with each other and with nothing else.
// Now both come from the shared format package, so these specs run against the
// real table definition.
func createTable(db *sql.DB) {
	GinkgoHelper()

	statement, err := enclikeystore.CreateTableSQL(enclikeystore.DriverSQLite)
	Expect(err).NotTo(HaveOccurred())
	_, err = db.Exec(statement)
	Expect(err).NotTo(HaveOccurred())
}

// sqliteDB opens a database of its own for one spec, on the real engine rather
// than an imitation of it: what this store does is issue SQL, so a fake would
// be testing the fake.
func sqliteDB() *sql.DB {
	GinkgoHelper()

	db, err := sql.Open("sqlite", filepath.Join(GinkgoT().TempDir(), "keys.db"))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(db.Close)

	createTable(db)
	return db
}

// insertKey writes a record the way whatever issues keys would.
func insertKey(db *sql.DB, key, subject, scopes string, expiresAt any) {
	GinkgoHelper()

	statement, err := enclikeystore.InsertSQL(enclikeystore.DriverSQLite)
	Expect(err).NotTo(HaveOccurred())
	_, err = db.Exec(statement, enclikeystore.Fingerprint(key), subject, scopes, expiresAt)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("the database-backed key store", func() {
	var (
		ctx context.Context
		db  *sql.DB
	)

	// store builds the key store over the open database, on a fixed clock so
	// that an expiry can be judged without waiting for one.
	store := func(now time.Time) authkit.KeyStore {
		GinkgoHelper()

		s, err := keystore.NewDB(db, enclikeystore.DriverSQLite,
			keystore.WithDBClock(func() time.Time { return now }))
		Expect(err).NotTo(HaveOccurred())
		return s
	}

	BeforeEach(func() {
		ctx = context.Background()
		db = sqliteDB()
	})

	It("returns the caller a key belongs to", func() {
		insertKey(db, "a-key", "payment-provider", "orders:write", nil)

		principal, err := store(time.Now()).Lookup(ctx, "a-key")

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.Subject).To(Equal("payment-provider"))
		Expect(principal.Scheme).To(Equal(authkit.SchemeAPIKey))
	})

	// Space-separated, the same way a JWT writes its scope claim, so that one
	// convention covers both kinds of credential.
	It("reads the permissions as a space-separated list", func() {
		insertKey(db, "a-key", "svc", "orders:read orders:write", nil)

		principal, err := store(time.Now()).Lookup(ctx, "a-key")

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.Scopes).To(Equal([]string{"orders:read", "orders:write"}))
	})

	It("gives a key with no permissions an empty scope list", func() {
		insertKey(db, "a-key", "svc", "", nil)

		principal, err := store(time.Now()).Lookup(ctx, "a-key")

		Expect(err).NotTo(HaveOccurred())
		Expect(principal.Scopes).To(BeEmpty())
	})

	It("reports a key it does not know", func() {
		_, err := store(time.Now()).Lookup(ctx, "not-a-key")

		Expect(err).To(MatchError(authkit.ErrKeyNotFound))
	})

	It("reports an empty key without asking the database", func() {
		_, err := store(time.Now()).Lookup(ctx, "")

		Expect(err).To(MatchError(authkit.ErrKeyNotFound))
	})

	// The whole point of the fingerprint: what is stored is not usable as a key.
	It("stores the key under its fingerprint, not as itself", func() {
		insertKey(db, "a-key", "svc", "", nil)

		var stored string
		Expect(db.QueryRow("SELECT " + enclikeystore.ColumnFingerprint + " FROM " + enclikeystore.TableName).Scan(&stored)).To(Succeed())
		Expect(stored).NotTo(Equal("a-key"))
		Expect(stored).To(Equal(enclikeystore.Fingerprint("a-key")))
	})

	Describe("expiry", func() {
		now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

		It("accepts a key with no deadline", func() {
			insertKey(db, "a-key", "svc", "", nil)

			_, err := store(now).Lookup(ctx, "a-key")

			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts a key whose deadline has not arrived", func() {
			insertKey(db, "a-key", "svc", "", now.Add(time.Hour))

			_, err := store(now).Lookup(ctx, "a-key")

			Expect(err).NotTo(HaveOccurred())
		})

		// An expired key is a definite answer, not a failure: the store was
		// asked and said no, which is a 401 rather than a 5xx.
		It("refuses a key past its deadline", func() {
			insertKey(db, "a-key", "svc", "", now.Add(-time.Hour))

			_, err := store(now).Lookup(ctx, "a-key")

			Expect(err).To(MatchError(authkit.ErrKeyNotFound))
		})

		// The two refusals mean the same thing to the caller and very different
		// things to whoever has to explain why a key stopped working.
		It("says that the key expired rather than that it is unknown", func() {
			insertKey(db, "a-key", "svc", "", now.Add(-time.Hour))

			_, err := store(now).Lookup(ctx, "a-key")

			Expect(err.Error()).To(ContainSubstring("expired"))
		})
	})

	// A record nobody can be identified by is a fault in the data, not a wrong
	// key. Reporting it as unknown would send the key's owner off to check a
	// key that is perfectly correct.
	It("does not blame the caller for a record with no subject", func() {
		insertKey(db, "a-key", "", "orders:write", nil)

		_, err := store(time.Now()).Lookup(ctx, "a-key")

		Expect(err).To(HaveOccurred())
		Expect(err).NotTo(MatchError(authkit.ErrKeyNotFound))
	})

	Describe("when the database cannot be reached", func() {
		// Reporting an outage as "no such key" answers 401, which tells every
		// caller in the system that their credential is bad at the moment
		// nothing can check any of them.
		It("does not report the failure as an unknown key", func() {
			Expect(db.Close()).To(Succeed())

			_, err := store(time.Now()).Lookup(ctx, "a-key")

			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(MatchError(authkit.ErrKeyNotFound))
		})

		It("does not put the key in the error", func() {
			Expect(db.Close()).To(Succeed())

			_, err := store(time.Now()).Lookup(ctx, "a-secret-key")

			Expect(err.Error()).NotTo(ContainSubstring("a-secret-key"))
		})
	})

	// The step that gets forgotten on a new environment is running
	// `encli auth keystore init`. Without this check the application starts
	// cleanly and answers 503 to the first caller presenting an API key, in
	// production, possibly days later.
	Describe("Ready", func() {
		It("passes when the table is there", func() {
			s, err := keystore.NewDB(db, enclikeystore.DriverSQLite)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.Ready(ctx)).To(Succeed())
		})

		It("fails when the table was never created", func() {
			_, err := db.Exec("DROP TABLE " + enclikeystore.TableName)
			Expect(err).NotTo(HaveOccurred())
			s, err := keystore.NewDB(db, enclikeystore.DriverSQLite)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.Ready(ctx)).To(HaveOccurred())
		})

		// The message has to name the step that was missed. Whoever sees it is
		// looking at a deployment that will not start, not at this package.
		It("names the command that creates the table", func() {
			_, err := db.Exec("DROP TABLE " + enclikeystore.TableName)
			Expect(err).NotTo(HaveOccurred())
			s, err := keystore.NewDB(db, enclikeystore.DriverSQLite)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.Ready(ctx)).To(MatchError(ContainSubstring("encli auth keystore init")))
		})

		// This is what the probe is for beyond a missing table: the two sides of
		// the format are in different repositories, and a column renamed on one
		// of them has to stop the application rather than a request.
		It("fails when a column the lookup reads is missing", func() {
			_, err := db.Exec("DROP TABLE " + enclikeystore.TableName)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`CREATE TABLE ` + enclikeystore.TableName + ` (
				key_fingerprint TEXT NOT NULL PRIMARY KEY,
				subject TEXT NOT NULL,
				scope TEXT NOT NULL,
				expires_at DATETIME,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
			Expect(err).NotTo(HaveOccurred())
			s, err := keystore.NewDB(db, enclikeystore.DriverSQLite)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.Ready(ctx)).To(HaveOccurred())
		})

		It("fails when the database cannot be reached", func() {
			s, err := keystore.NewDB(db, enclikeystore.DriverSQLite)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Close()).To(Succeed())

			Expect(s.Ready(ctx)).To(HaveOccurred())
		})

		// The probe must never match a key somebody holds.
		It("looks up a value no key can fingerprint to", func() {
			_, err := db.Exec(
				"INSERT INTO " + enclikeystore.TableName +
					" (" + enclikeystore.ColumnFingerprint + ", " + enclikeystore.ColumnSubject + ", " +
					enclikeystore.ColumnScopes + ") VALUES ('readiness-probe','svc','')")
			Expect(err).NotTo(HaveOccurred())
			s, err := keystore.NewDB(db, enclikeystore.DriverSQLite)
			Expect(err).NotTo(HaveOccurred())

			// A row under that value is somebody else's doing, and the probe
			// still reports the table as readable.
			Expect(s.Ready(ctx)).To(Succeed())
			Expect(enclikeystore.Fingerprint("readiness-probe")).NotTo(Equal("readiness-probe"))
		})
	})

	Describe("NewDB", func() {
		// The placeholder syntax is the only thing the driver decides here, and
		// getting it wrong makes every lookup a syntax error at run time.
		DescribeTable("accepts every driver the table can be created on",
			func(driver string) {
				_, err := keystore.NewDB(db, driver)

				Expect(err).NotTo(HaveOccurred())
			},
			Entry("PostgreSQL", enclikeystore.DriverPostgres),
			Entry("MySQL", enclikeystore.DriverMySQL),
			Entry("SQLite", enclikeystore.DriverSQLite),
		)

		It("refuses a driver it cannot write a statement for", func() {
			_, err := keystore.NewDB(db, "oracle")

			Expect(err).To(MatchError(ContainSubstring("oracle")))
		})

		It("refuses to read keys from nowhere", func() {
			_, err := keystore.NewDB(nil, enclikeystore.DriverSQLite)

			Expect(err).To(HaveOccurred())
		})
	})
})
