package keystore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/keystore"
	_ "modernc.org/sqlite"
)

// createTableSQLite mirrors what `encli auth keystore init` creates on SQLite.
//
// ⚠ It is written out rather than imported: encli is a separate repository, so
// the table definition and the query that reads it cannot be checked against
// each other by a compiler. Copying it here is what makes these specs fail if
// the two drift — which is the only mechanism available.
const createTableSQLite = `
CREATE TABLE IF NOT EXISTS ensoria_api_keys (
    key_fingerprint TEXT NOT NULL PRIMARY KEY,
    subject TEXT NOT NULL,
    scopes TEXT NOT NULL,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// sqliteDB opens a database of its own for one spec, on the real engine rather
// than an imitation of it: what this store does is issue SQL, so a fake would
// be testing the fake.
func sqliteDB() *sql.DB {
	GinkgoHelper()

	db, err := sql.Open("sqlite", filepath.Join(GinkgoT().TempDir(), "keys.db"))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(db.Close)

	_, err = db.Exec(createTableSQLite)
	Expect(err).NotTo(HaveOccurred())
	return db
}

// insertKey writes a record the way whatever issues keys would.
func insertKey(db *sql.DB, key, subject, scopes string, expiresAt any) {
	GinkgoHelper()

	_, err := db.Exec(
		`INSERT INTO ensoria_api_keys (key_fingerprint, subject, scopes, expires_at) VALUES (?, ?, ?, ?)`,
		keystore.Fingerprint(key), subject, scopes, expiresAt)
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

		s, err := keystore.NewDB(db, keystore.DriverSQLite,
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
		Expect(db.QueryRow(`SELECT key_fingerprint FROM ensoria_api_keys`).Scan(&stored)).To(Succeed())
		Expect(stored).NotTo(Equal("a-key"))
		Expect(stored).To(Equal(keystore.Fingerprint("a-key")))
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

	Describe("NewDB", func() {
		// The placeholder syntax is the only thing the driver decides here, and
		// getting it wrong makes every lookup a syntax error at run time.
		DescribeTable("accepts every driver the table can be created on",
			func(driver string) {
				_, err := keystore.NewDB(db, driver)

				Expect(err).NotTo(HaveOccurred())
			},
			Entry("PostgreSQL", keystore.DriverPostgres),
			Entry("MySQL", keystore.DriverMySQL),
			Entry("SQLite", keystore.DriverSQLite),
		)

		It("refuses a driver it cannot write a statement for", func() {
			_, err := keystore.NewDB(db, "oracle")

			Expect(err).To(MatchError(ContainSubstring("oracle")))
		})

		It("refuses to read keys from nowhere", func() {
			_, err := keystore.NewDB(nil, keystore.DriverSQLite)

			Expect(err).To(HaveOccurred())
		})
	})
})
