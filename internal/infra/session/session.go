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
		settings, err := sessionSettings()
		if err != nil {
			return nil, err
		}
		if settings == nil {
			return nil, nil
		}

		sessionCfg, err := sessionkit.NewConfig(settings)
		if err != nil {
			return nil, err
		}
		cache, err := backingStore(lc, *envVal, settings)
		if err != nil {
			return nil, err
		}
		return sessionkit.NewStore(cache, sessionCfg)
	}
}

// NewSessionCookies builds the writer for the session cookie, or nothing.
//
// Nil means the same as a nil Store: AUTH_SESSION_STORE is unset, so no cookie
// is ever written. The endpoints that trade a token for a session take both,
// and the startup checks are what stop an application from serving them with
// neither.
//
// It is a second constructor rather than a field on the store because the two
// are wanted in different places: the verifier and the exchange endpoints write
// cookies, and only the endpoints create sessions.
func NewSessionCookies() (*sessionkit.Cookies, error) {
	settings, err := sessionSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, nil
	}

	sessionCfg, err := sessionkit.NewConfig(settings)
	if err != nil {
		return nil, err
	}
	return sessionkit.NewCookies(sessionCfg), nil
}

// sessionSettings reads the session settings, or nil when the application does
// not authenticate browsers with a cookie.
func sessionSettings() (*appconfig.AuthSession, error) {
	params, err := registry.ModuleParams(defaultModule)
	if err != nil {
		return nil, fmt.Errorf("session: reading the %s configuration: %w", defaultModule, err)
	}
	if params.Auth == nil {
		return nil, nil
	}
	return params.Auth.Session, nil
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
