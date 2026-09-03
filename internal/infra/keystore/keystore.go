// Package keystore connects the framework's built-in API key store to the
// storage AUTH_KEYSTORE names.
//
// The store itself is plamo/keystore; this is the wiring, which is why it sits
// under infra alongside the other things that own a connection.
package keystore

import (
	"fmt"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	infracache "github.com/ensoria/ensoria-template/internal/infra/cache"
	infradb "github.com/ensoria/ensoria-template/internal/infra/db"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/keystore"
)

// defaultModule is the configuration module the auth settings are read from.
// The key store belongs to the application rather than to one module.
const defaultModule = "default"

// NewAPIKeyStore builds the built-in API key store, or nothing.
//
// Nil is the ordinary answer, not a failure: AUTH_KEYSTORE unset means the
// application either does not use API keys or verifies them with a store of its
// own. Whoever receives the nil decides what to do with it — see
// app/auth.NewVerifier, which falls back to the keys in the configuration.
//
// The connection is dialed at startup rather than on the first request that
// presents a key, which is what makes an unreachable key store stop the
// application instead of refusing callers one at a time.
func NewAPIKeyStore(envVal *string) func(lc dikit.LC) (authkit.KeyStore, error) {
	return func(lc dikit.LC) (authkit.KeyStore, error) {
		params, err := registry.ModuleParams(defaultModule)
		if err != nil {
			return nil, fmt.Errorf("keystore: reading the %s configuration: %w", defaultModule, err)
		}
		return build(lc, params.Auth)
	}
}

// build selects the backend AUTH_KEYSTORE named.
func build(lc dikit.LC, cfg *appconfig.Auth) (authkit.KeyStore, error) {
	if cfg == nil || cfg.KeyStore == nil {
		return nil, nil
	}

	switch {
	case cfg.KeyStore.Redis != nil:
		return keystore.NewRedis(infracache.NewKeyStoreCache(lc, cfg.KeyStore.Redis))

	case cfg.KeyStore.DB != nil:
		// The table is not part of the application's schema and no migration
		// carries it, so it is created by `encli auth keystore init`. Starting
		// without it fails on the first lookup rather than here: the connection
		// is verified at startup, but asking whether a table exists would mean
		// a second dialect-specific query for a condition the first refused key
		// already reports.
		conn, err := infradb.NewKeyStoreDB(lc, cfg.KeyStore.DB)
		if err != nil {
			return nil, err
		}
		return keystore.NewDB(conn, cfg.KeyStore.DB.Driver)

	default:
		// AuthKeyStore is built by the configuration package, which only ever
		// produces one of the two above. Reaching here means that changed.
		return nil, fmt.Errorf("keystore: AUTH_KEYSTORE selected no backend")
	}
}
