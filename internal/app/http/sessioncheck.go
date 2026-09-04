package http

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// The configuration keys these checks name. They are written out because the
// messages exist to be acted on, and a message that describes a setting without
// naming it leaves the reader to guess which one.
const (
	sessionStoreKey     = "AUTH_SESSION_STORE"
	sessionKeyPrefix    = "AUTH_SESSION_"
	cookieInsecureKey   = "AUTH_SESSION_COOKIE_INSECURE"
	corsAllowOriginKey  = "CORS_ALLOW_ORIGIN"
	corsAllowCredsKey   = "CORS_ALLOW_CREDENTIALS"
	wildcardAllowOrigin = "*"
)

// checkSessionConfiguration reports a cookie-authentication setup that cannot
// work, or that works in a way nobody would have chosen deliberately.
//
// Each of the four below is silent in its own way if left alone: a setting that
// has no effect, an endpoint that answers 503 to every sign-in, a cookie any
// site can make the browser send, or one that travels in clear text. None of
// them shows up as a crash, and the first three do not even show up as an
// error — which is why they are settled here, at startup, while the person who
// can fix them is watching.
//
// It returns an error rather than stopping the process itself so that the rules
// can be tested.
func checkSessionConfiguration(envVal string, params *appconfig.Parameters, modules []*rest.Module) error {
	if params == nil {
		return nil
	}

	session := sessionSettings(params.Auth)
	if err := checkSessionKeysDoSomething(session, params.AllValues); err != nil {
		return err
	}
	if err := checkSessionEndpointsAreServable(session, modules); err != nil {
		return err
	}
	if session == nil {
		return nil
	}
	if err := checkTrustedOrigins(session, params.CORS); err != nil {
		return err
	}
	return checkCookieIsSecure(envVal, session)
}

// sessionSettings reads the session settings, or nil when the application does
// not authenticate browsers with a cookie. appconfig answers the same question
// as a bool (Auth.UsesSessions); this needs the settings themselves.
func sessionSettings(auth *appconfig.Auth) *appconfig.AuthSession {
	if auth == nil {
		return nil
	}
	return auth.Session
}

// checkSessionKeysDoSomething reports a configuration that tunes sessions
// without turning them on.
//
// AUTH_SESSION_STORE is the switch, so every other AUTH_SESSION_ key is read
// only when it is set. Writing AUTH_SESSION_IDLE_TTL alone is not an error the
// configuration package can report — the value parses, it is simply never
// looked at — and the result is a deployment whose author believes sessions
// expire after a day and has no sessions at all.
func checkSessionKeysDoSomething(session *appconfig.AuthSession, values map[string]string) error {
	if session != nil {
		return nil
	}

	var ignored []string
	for key, value := range values {
		if key == sessionStoreKey || value == "" || !strings.HasPrefix(key, sessionKeyPrefix) {
			continue
		}
		ignored = append(ignored, key)
	}
	if len(ignored) == 0 {
		return nil
	}
	sort.Strings(ignored)

	return fmt.Errorf("%s is not set, so browser sessions are off and these keys do nothing: %s. "+
		"Set %s=%s to turn cookie authentication on, or remove the keys",
		sessionStoreKey, strings.Join(ignored, ", "),
		sessionStoreKey, appconfig.AuthSessionStoreRedis)
}

// checkSessionEndpointsAreServable reports endpoints that were registered
// without the store they need.
//
// The generic check next door would catch this too, as "endpoints accept only
// the session credential, which nothing verifies" — which describes the symptom
// and sends the reader to widen a security declaration. This one names the
// wiring: either the store is missing or the endpoints are.
func checkSessionEndpointsAreServable(session *appconfig.AuthSession, modules []*rest.Module) error {
	if session != nil {
		return nil
	}
	if !slices.Contains(restkit.DeclaredSchemes(modules), authkit.SchemeSession) {
		return nil
	}

	return fmt.Errorf("endpoints authenticate with a session cookie but %s is not set, "+
		"so no session can be created or resolved and every one of them would refuse every caller. "+
		"Set %s=%s, or stop registering the session endpoints "+
		"(the blank import of internal/app/auth/api in internal/app/bootstrap/server)",
		sessionStoreKey, sessionStoreKey, appconfig.AuthSessionStoreRedis)
}

// checkTrustedOrigins refuses cookie authentication with a wildcard origin.
//
// ⚠ This is the combination that gives away what the session cookie is for. The
// browser attaches it to a cross-site request whether or not anyone meant to
// send it, and CORS_ALLOW_ORIGIN is where this application says which other
// origin is its own frontend — so a wildcard says every site is, and the
// cross-origin check has nothing left to refuse.
//
// Browsers refuse the same combination themselves: a response cannot carry both
// Access-Control-Allow-Origin: * and Access-Control-Allow-Credentials: true. So
// a deployment that reached here was going to fail anyway, in the browser,
// where the error is a console message rather than a startup failure.
func checkTrustedOrigins(session *appconfig.AuthSession, cors *appconfig.CORS) error {
	if strings.TrimSpace(cors.AllowOrigin()) != wildcardAllowOrigin {
		return nil
	}

	return fmt.Errorf("cookie authentication is on (%s=%s) and %s is %q, which cannot be combined: "+
		"a wildcard says every site is this application's frontend, and the browser attaches the "+
		"session cookie to their requests too. "+
		"Serving the frontend from this same origin needs no CORS at all — leave %s unset. "+
		"Serving it from another origin needs that origin written out "+
		"(%s=https://app.example.com, comma-separated for several) together with %s=true, "+
		"which a browser refuses to combine with %q in any case",
		sessionStoreKey, session.Store, corsAllowOriginKey, wildcardAllowOrigin,
		corsAllowOriginKey,
		corsAllowOriginKey, corsAllowCredsKey, wildcardAllowOrigin)
}

// checkCookieIsSecure refuses a session cookie sent over plain HTTP anywhere it
// could reach a real user.
//
// The setting exists because Safari has no localhost exemption and a phone on a
// LAN address has none either, so developing over http:// needs it. Nothing
// else does, and a deployment that has it set is sending the credential in
// clear text to anything on the path.
func checkCookieIsSecure(envVal string, session *appconfig.AuthSession) error {
	if session.CookieSecure() || cookieInsecureAllowed(envVal) {
		return nil
	}

	return fmt.Errorf("%s=true drops the Secure attribute from the session cookie, "+
		"which only makes sense while developing over plain HTTP (this is the %q environment): "+
		"the cookie would travel unencrypted and anything on the network could read it and "+
		"sign in as its owner. Remove the key, or set it only in the %s and %s configurations",
		cookieInsecureKey, envVal, env.Local, env.Test)
}

// cookieInsecureAllowed reports whether the Secure attribute may be dropped.
// Only the environments a developer runs on their own machine qualify.
func cookieInsecureAllowed(envVal string) bool {
	switch env.Environment(envVal) {
	case env.Local, env.Test:
		return true
	default:
		return false
	}
}
