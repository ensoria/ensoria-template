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
		// The selector resolved, so the application asked for a key store and
		// would otherwise start with none — refusing every API key at run time,
		// which is exactly the silence this whole store was added to end.
		return nil, fmt.Errorf(
			"keystore: AUTH_KEYSTORE=%s is not implemented yet: the built-in store reads keys "+
				"from Redis only. Set AUTH_KEYSTORE=%s, or set AUTH_API_KEYS_EXTERNAL=true and "+
				"hand the verifier an authkit.KeyStore backed by your database",
			appconfig.AuthKeyStoreDB, appconfig.AuthKeyStoreRedis)

	default:
		// AuthKeyStore is built by the configuration package, which only ever
		// produces one of the two above. Reaching here means that changed.
		return nil, fmt.Errorf("keystore: AUTH_KEYSTORE selected no backend")
	}
}
