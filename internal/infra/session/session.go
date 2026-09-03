// Package session connects the browser session store to the storage
// AUTH_SESSION_STORE names.
//
// The store itself is plamo/sessionkit; this is the wiring, which is why it
// sits under infra alongside the other things that own a connection.
package session

import (
	"fmt"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/config/pkg/registry"
	infracache "github.com/ensoria/ensoria-template/internal/infra/cache"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

// defaultModule is the configuration module the auth settings are read from.
// Sessions belong to the application rather than to one module.
const defaultModule = "default"

// sessionKeyPrefix namespaces the session records within their Redis database.
const sessionKeyPrefix = "session"

// NewSessionStore builds the browser session store, or nothing.
//
// Nil is the ordinary answer, not a failure: AUTH_SESSION_STORE unset means the
// application does not authenticate browsers with a cookie. Whoever receives the
// nil decides what to do with it — the verifier refuses to start if the
// configuration says otherwise.
func NewSessionStore(envVal *string) func(lc dikit.LC) (sessionkit.Store, error) {
	return func(lc dikit.LC) (sessionkit.Store, error) {
		params, err := registry.ModuleParams(defaultModule)
		if err != nil {
			return nil, fmt.Errorf("session: reading the %s configuration: %w", defaultModule, err)
		}
		return build(lc, *envVal, params.Auth)
	}
}

// build selects the backend AUTH_SESSION_STORE named.
func build(lc dikit.LC, envVal string, cfg *appconfig.Auth) (sessionkit.Store, error) {
	if cfg == nil || cfg.Session == nil {
		return nil, nil
	}

	sessionCfg, err := sessionkit.NewConfig(cfg.Session)
	if err != nil {
		return nil, err
	}

	cache, err := backingStore(lc, envVal, cfg.Session)
	if err != nil {
		return nil, err
	}
	return sessionkit.NewStore(cache, sessionCfg)
}

// backingStore opens the storage the selected backend keeps sessions in.
func backingStore(lc dikit.LC, envVal string, cfg *appconfig.AuthSession) (enscache.Cache, error) {
	switch cfg.Store {
	case appconfig.AuthSessionStoreRedis:
		return infracache.NewSessionCache(lc, cfg.Redis), nil

	case appconfig.AuthSessionStoreMemory:
		if !memoryAllowed(envVal) {
			return nil, fmt.Errorf(
				"session: AUTH_SESSION_STORE=%s only runs in the %s and %s environments (got %q): "+
					"sessions kept in the process are not shared between them, so signing out reaches "+
					"one node and the next request is still signed in — which is the guarantee a "+
					"server-side session store is chosen for. Set AUTH_SESSION_STORE=%s",
				appconfig.AuthSessionStoreMemory, env.Local, env.Test, envVal,
				appconfig.AuthSessionStoreRedis)
		}
		return cachememory.New(sessionKeyPrefix), nil

	default:
		// AuthSession is built by the configuration package, which only ever
		// produces one of the two above. Reaching here means that changed.
		return nil, fmt.Errorf("session: AUTH_SESSION_STORE selected no backend")
	}
}

// memoryAllowed reports whether the in-process store may be used. Only the
// environments a developer runs on their own machine qualify.
func memoryAllowed(envVal string) bool {
	switch env.Environment(envVal) {
	case env.Local, env.Test:
		return true
	default:
		return false
	}
}
