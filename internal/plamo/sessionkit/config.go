package sessionkit

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ensoria/config/pkg/appconfig"
)

// hostCookiePrefix is the cookie name prefix browsers enforce attributes for.
// A cookie named with it is refused outright unless it is Secure, has Path=/
// and names no Domain — which is what makes it worth defaulting to, and what
// makes it incompatible with serving over plain HTTP.
const hostCookiePrefix = "__Host-"

// cookiePath is the path every session cookie is written under. It is fixed:
// hostCookiePrefix requires it, and narrowing it only produces a cookie that is
// missing from the requests that needed it.
const cookiePath = "/"

// idleRefreshDivisor decides how rarely resolving a session rewrites it.
//
// The idle deadline has to move forward as a session is used, and writing on
// every request would put a write in front of every authenticated request for a
// deadline measured in days. Instead the record is rewritten once the session
// is more than IdleTTL/idleRefreshDivisor past its last write.
//
// ⚠ The cost is paid in accuracy, in the safe direction: a session can be
// collected up to one such interval before its idle deadline, never after it.
// With the divisor at 10 that is at most 10% early.
const idleRefreshDivisor = 10

// Config is what this package needs from the application's configuration,
// resolved once so that nothing downstream has to interpret a setting again.
type Config struct {
	// CookieName is the name the session id is carried under.
	CookieName string
	// CookieSameSite is the cookie's SameSite attribute.
	CookieSameSite http.SameSite
	// CookieSecure marks the cookie as HTTPS-only. It is true everywhere the
	// application is deployed; false is a local-development setting.
	CookieSecure bool

	// AbsoluteTTL and PersistentAbsoluteTTL are the two lifetime profiles.
	AbsoluteTTL           time.Duration
	PersistentAbsoluteTTL time.Duration
	// IdleTTL is how long a session survives without being used, shared by both
	// profiles.
	IdleTTL time.Duration
}

// NewConfig resolves the application's session settings.
//
// It returns an error for a combination that cannot work, so that a deployment
// stops at startup rather than serving an application nobody can sign in to.
// The settings have already been checked for shape by the configuration package;
// what is left here are the rules that need the cookie's own vocabulary.
func NewConfig(cfg *appconfig.AuthSession) (*Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("sessionkit: sessions are not configured: set AUTH_SESSION_STORE")
	}

	sameSite, err := sameSiteOf(cfg.CookieSameSite)
	if err != nil {
		return nil, err
	}

	resolved := &Config{
		CookieName:            cfg.CookieName,
		CookieSameSite:        sameSite,
		CookieSecure:          cfg.CookieSecure(),
		AbsoluteTTL:           cfg.AbsoluteTTL,
		PersistentAbsoluteTTL: cfg.PersistentAbsoluteTTL,
		IdleTTL:               cfg.IdleTTL,
	}
	if err := resolved.Validate(); err != nil {
		return nil, err
	}
	return resolved, nil
}

// Validate reports a configuration that cannot produce a working session.
//
// It is exported so that the startup checks can put the failure where the
// person who can fix it is watching, rather than leaving it to be discovered as
// sign-in mysteriously never taking effect.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("sessionkit: no session configuration")
	}
	if c.CookieName == "" {
		return fmt.Errorf("sessionkit: the session cookie has no name: set AUTH_SESSION_COOKIE_NAME")
	}

	// This one is worth stopping for because nothing else would report it. The
	// browser refuses a __Host- cookie that is not Secure, and it refuses it
	// silently: the response looks fine, the cookie is simply never stored, and
	// every sign-in appears to succeed and then not have happened.
	if strings.HasPrefix(c.CookieName, hostCookiePrefix) && !c.CookieSecure {
		return fmt.Errorf(
			"sessionkit: the session cookie is named %q but Secure is off "+
				"(AUTH_SESSION_COOKIE_INSECURE=true): browsers refuse a %s cookie without Secure, "+
				"so no session would ever be stored. Either drop AUTH_SESSION_COOKIE_INSECURE, "+
				"or set AUTH_SESSION_COOKIE_NAME to a name without the %s prefix while developing "+
				"over plain HTTP",
			c.CookieName, hostCookiePrefix, hostCookiePrefix)
	}

	for _, ttl := range []struct {
		name  string
		key   string
		value time.Duration
	}{
		{"the absolute lifetime", "AUTH_SESSION_ABSOLUTE_TTL", c.AbsoluteTTL},
		{"the persistent absolute lifetime", "AUTH_SESSION_PERSISTENT_ABSOLUTE_TTL", c.PersistentAbsoluteTTL},
		{"the idle limit", "AUTH_SESSION_IDLE_TTL", c.IdleTTL},
	} {
		if ttl.value <= 0 {
			return fmt.Errorf("sessionkit: %s is %s: set %s to a positive duration",
				ttl.name, ttl.value, ttl.key)
		}
	}
	return nil
}

// AbsoluteTTLFor returns the lifetime of the profile asked for.
func (c *Config) AbsoluteTTLFor(persistent bool) time.Duration {
	if persistent {
		return c.PersistentAbsoluteTTL
	}
	return c.AbsoluteTTL
}

// maxAbsoluteTTL is the longest any session can live. It bounds how long the
// store has to remember that a subject's sessions were revoked.
func (c *Config) maxAbsoluteTTL() time.Duration {
	return max(c.AbsoluteTTL, c.PersistentAbsoluteTTL)
}

// idleRefreshInterval is how long resolving a session may leave its record
// unwritten before the idle deadline is moved forward.
func (c *Config) idleRefreshInterval() time.Duration {
	return c.IdleTTL / idleRefreshDivisor
}

// sameSiteOf maps the configured SameSite onto the one net/http writes.
//
// The vocabulary is closed and the configuration package already refuses
// anything outside it, so this is the second line rather than the first: it
// exists because a Config can also be built in code, and a zero SameSite would
// otherwise write no attribute at all — which is a cookie with none of the
// protection this one is chosen for.
func sameSiteOf(value string) (http.SameSite, error) {
	switch value {
	case appconfig.AuthSessionSameSiteLax:
		return http.SameSiteLaxMode, nil
	case appconfig.AuthSessionSameSiteStrict:
		return http.SameSiteStrictMode, nil
	default:
		return 0, fmt.Errorf("sessionkit: unknown SameSite %q: expected %q or %q",
			value, appconfig.AuthSessionSameSiteLax, appconfig.AuthSessionSameSiteStrict)
	}
}
