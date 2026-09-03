package keystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// The table the built-in key store reads, and the columns it reads from it.
//
// ⚠ These names are a contract with `encli auth keystore init`, which creates
// the table, and they live in a different repository. Renaming one here without
// renaming it there produces a store that connects successfully and then fails
// every lookup with a SQL error. There is no compiler between the two, so a
// change to either side has to be made to both.
const (
	apiKeysTable      = "ensoria_api_keys"
	columnFingerprint = "key_fingerprint"
	columnSubject     = "subject"
	columnScopes      = "scopes"
	columnExpiresAt   = "expires_at"
)

// The driver names, as the configuration spells them (appconfig normalizes
// "sqlite3" to "sqlite" before they reach here).
const (
	DriverPostgres = "postgres"
	DriverMySQL    = "mysql"
	DriverSQLite   = "sqlite"
)

// dbStore answers key lookups from a SQL database.
//
// ⚠ Withdrawing a key has to take effect everywhere at once, so nothing here
// caches a row. The lookup is a primary-key read of one narrow row, which is
// the cheapest thing a database does; a cache in front of it would buy very
// little and would keep a deleted key working on whichever node still held it.
type dbStore struct {
	db    *sql.DB
	query string
	now   func() time.Time
}

// DBOption adjusts a database-backed store. There is one, and it exists so that
// a test can decide what "now" is when judging an expiry.
type DBOption func(*dbStore)

// WithDBClock replaces the clock the store compares expiry against. Passing nil
// leaves it alone.
func WithDBClock(now func() time.Time) DBOption {
	return func(s *dbStore) {
		if now != nil {
			s.now = now
		}
	}
}

// NewDB reads keys from the table `encli auth keystore init` creates.
//
// driver decides the placeholder syntax and nothing else; the statement is
// built once here rather than at every lookup.
func NewDB(db *sql.DB, driver string, opts ...DBOption) (authkit.KeyStore, error) {
	if db == nil {
		return nil, errors.New("keystore: no database to read keys from")
	}
	placeholder, err := placeholderFor(driver)
	if err != nil {
		return nil, err
	}

	store := &dbStore{
		db: db,
		query: fmt.Sprintf("SELECT %s, %s, %s FROM %s WHERE %s = %s",
			columnSubject, columnScopes, columnExpiresAt, apiKeysTable, columnFingerprint, placeholder),
		now: time.Now,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store, nil
}

// placeholderFor returns how the driver spells the first bind parameter.
func placeholderFor(driver string) (string, error) {
	switch driver {
	case DriverPostgres:
		return "$1", nil
	case DriverMySQL, DriverSQLite:
		return "?", nil
	default:
		return "", fmt.Errorf("keystore: cannot read keys from a %q database: expected %q, %q or %q",
			driver, DriverPostgres, DriverMySQL, DriverSQLite)
	}
}

// Lookup returns the caller a key belongs to.
//
// The key never reaches the database or an error message: only its fingerprint
// does. An unknown or expired key is reported as authkit.ErrKeyNotFound, and
// everything else as an error meaning the store could not be asked — the
// difference between one caller being told no and every caller being told no
// during an outage.
func (s *dbStore) Lookup(ctx context.Context, key string) (*authkit.Principal, error) {
	if key == "" {
		return nil, authkit.ErrKeyNotFound
	}

	fingerprint := Fingerprint(key)
	var (
		subject   string
		scopes    string
		expiresAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, s.query, fingerprint).Scan(&subject, &scopes, &expiresAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, authkit.ErrKeyNotFound
	case err != nil:
		return nil, fmt.Errorf("keystore: reading the key record %s: %w", short(fingerprint), err)
	}

	// An expired key is a definite answer rather than a failure: the store was
	// asked and said no. It is kept apart from "no such key" only in the error
	// text, because the two mean the same thing to the caller and very
	// different things to whoever has to explain the refusal.
	if expiresAt.Valid && !expiresAt.Time.After(s.now().UTC()) {
		return nil, fmt.Errorf("%w: the key expired at %s",
			authkit.ErrKeyNotFound, expiresAt.Time.UTC().Format(time.RFC3339))
	}
	if subject == "" {
		return nil, fmt.Errorf("%w (fingerprint %s)", errUnusableRecord, short(fingerprint))
	}

	return &authkit.Principal{
		Subject: subject,
		Scopes:  parseScopes(scopes),
		Scheme:  authkit.SchemeAPIKey,
	}, nil
}

// parseScopes reads the stored permissions.
//
// They are space-separated, the same way a JWT writes its scope claim (RFC
// 8693), so that one convention covers both kinds of credential and a reader
// familiar with one is not surprised by the other.
func parseScopes(scopes string) []string {
	return strings.Fields(scopes)
}
