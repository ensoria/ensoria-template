package authkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidCredential means a credential was presented and could not be
// trusted. That always ends the request with 401, even on a public endpoint,
// because silently ignoring a broken credential hides bugs from the caller.
//
// ErrCredentialUnavailable means the credential could not be judged at all,
// because whatever holds the answer could not be reached.
//
// ⚠ The second is not a variety of the first, and the difference is the whole
// reason it exists. A credential store that is down says nothing about the
// credential: answering 401 would tell every caller in the system that their
// perfectly good credential is bad, at the exact moment nothing can verify
// anything. That is a 5xx — the request failed on this side and retrying it
// later is the right thing to do.
//
// A request carrying no credential at all is not an error. It comes back as a
// VerifyResult with no Principal, because a public endpoint is served without
// one and an endpoint that needs a caller answers 401 on its own declaration.
var (
	ErrInvalidCredential     = errors.New("credential could not be verified")
	ErrCredentialUnavailable = errors.New("the credential could not be checked")
)

// The labels on the records this package writes. They are constants so that a
// log platform can be given a search and an alert condition that survives a
// change of wording, and so that a test and that condition read the same value.
//
// ⚠ The two are deliberately separate types rather than one type with a reason
// field. A stale cookie is ordinary — a session expired, or somebody signed out
// on another device — and nobody should ever be paged for it. A store that
// cannot be reached is an outage. Sharing a type would make every alert depend
// on getting a filter right.
const (
	LogTypeSessionRejected         = "session_rejected_log"
	LogTypeSessionStoreUnavailable = "session_store_unavailable_log"
)

// ErrKeyNotFound is how a KeyStore reports that a key is not one it knows.
//
// ⚠ It is the only benign outcome a KeyStore may report; every other error is
// taken to mean the store could not be asked, and produces a 5xx rather than a
// 401. A store must therefore never report an outage as this error. The reverse
// mistake is safe: an unrecognized error is read as "could not ask", which is
// the cautious answer.
var ErrKeyNotFound = errors.New("authkit: no such API key")

// authorizationHeader and bearerPrefix locate a JWT on the request.
const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// scopeClaim holds the space-separated permissions of a token (RFC 8693).
const scopeClaim = "scope"

// apiKeySubjectPrefix names the caller behind an accepted API key. API keys
// identify a service rather than a person, and the built-in store keeps no
// owner information, so the subject is derived from the scheme.
const apiKeySubjectPrefix = "apikey"

// VerifyResult is what verification concluded about a request.
//
// It is a struct rather than a bare Principal because a verdict can carry an
// instruction as well as an identity. A browser holding a cookie for a session
// that no longer exists is anonymous — and should be told to stop sending it,
// or it will present the same dead cookie on every request for weeks.
type VerifyResult struct {
	// Principal is the caller, or nil when the request carried no credential
	// this verifier could turn into one. Nil is an ordinary outcome: a public
	// endpoint is served without a caller.
	Principal *Principal
	// Discard are cookies the browser should drop, written with Max-Age=0.
	//
	// ⚠ Only a credential the browser sends by itself belongs here. A cookie is
	// attached to every request whether or not anyone meant to send it, so a
	// dead one has to be taken back; a token or an API key was put on the
	// request deliberately, and answering 401 tells its owner what they need to
	// know.
	Discard []*http.Cookie
}

// Verifier turns the credentials on a request into a Principal.
type Verifier interface {
	// Verify reports who the request authenticated as.
	//
	// A request carrying no credential comes back as a result with no
	// Principal and no error. An error wrapping ErrInvalidCredential means a
	// credential was presented and cannot be trusted; one wrapping
	// ErrCredentialUnavailable means nothing could be concluded, because
	// whatever holds the answer did not respond.
	//
	// ⚠ The result is non-nil whenever the error is nil, and its Discard has to
	// be carried onto the response even when there is no Principal — that is
	// the whole case it exists for.
	Verify(r *rest.Request) (*VerifyResult, error)
	// Schemes reports which kinds of credential this verifier can check, in a
	// stable order (SchemeJWT, SchemeAPIKey, SchemeSession).
	//
	// It is the verifier, not the configuration, that knows the answer: a key
	// store injected by application code never appears in the configuration.
	// The startup checks ask for it so that an application verifying API keys
	// against a database is not mistaken for one that cannot verify them at all.
	Schemes() []string
}

// KeyStore resolves an API key into the caller it belongs to.
// Applications keeping keys in a database implement this and inject it, which
// is why key lookup is an interface rather than a fixed table.
//
// A key that is not known is reported as ErrKeyNotFound, and anything else as
// an error meaning the store could not be reached. See ErrKeyNotFound for why
// the difference decides between a 401 and a 5xx.
//
// It takes a context because a store worth having is usually across a network,
// and a lookup that cannot be cancelled outlives the request that wanted it.
type KeyStore interface {
	Lookup(ctx context.Context, key string) (*Principal, error)
}

// KeyStoreFunc adapts a plain function to KeyStore.
type KeyStoreFunc func(ctx context.Context, key string) (*Principal, error)

func (f KeyStoreFunc) Lookup(ctx context.Context, key string) (*Principal, error) {
	return f(ctx, key)
}

// verifier verifies a JWT, an API key, a session cookie, or any combination of
// them, depending on what is configured.
type verifier struct {
	jwtKeyfunc   jwt.Keyfunc
	parserOpts   []jwt.ParserOption
	apiKeyHeader string
	keys         KeyStore
	sessions     sessionkit.Store
	cookies      *sessionkit.Cookies
}

// NewVerifier builds the verifier described by the configuration.
//
// keys overrides where API keys are looked up; pass nil to use the keys from the
// configuration. sessions is where browser sessions are kept, and is required
// exactly when the configuration turns them on. Returns an error when the
// configuration cannot be satisfied (a shared-secret setup with no secret, for
// instance), so that a misconfiguration is caught at startup rather than on the
// first request.
func NewVerifier(cfg *appconfig.Auth, keys KeyStore, sessions sessionkit.Store) (Verifier, error) {
	if cfg == nil {
		return nil, errors.New("authkit: no auth configuration")
	}

	v := &verifier{apiKeyHeader: cfg.APIKeyHeader, keys: keys, sessions: sessions}
	if v.apiKeyHeader == "" {
		v.apiKeyHeader = appconfig.DefaultAPIKeyHeader
	}
	if v.keys == nil && len(cfg.APIKeys) > 0 {
		v.keys = staticKeyStore(cfg.APIKeys)
	}

	if err := v.configureSessions(cfg); err != nil {
		return nil, err
	}
	if err := v.configureJWT(cfg); err != nil {
		return nil, err
	}
	return v, nil
}

// configureSessions prepares the cookie half of verification.
//
// A configuration that turns sessions on without a store is refused here rather
// than left to surface later: the endpoints declaring Schemes: [session] would
// otherwise fail the startup check with a message about a scheme nothing
// verifies, which describes the symptom rather than the wiring mistake.
func (v *verifier) configureSessions(cfg *appconfig.Auth) error {
	if cfg.Session == nil {
		if v.sessions != nil {
			return errors.New("authkit: a session store was given but AUTH_SESSION_STORE is not set, " +
				"so nothing would ever read it: set the selector, or stop building the store")
		}
		return nil
	}
	if v.sessions == nil {
		return errors.New("authkit: AUTH_SESSION_STORE is set but no session store was given: " +
			"build one with sessionkit.NewStore and hand it to the verifier")
	}

	sessionCfg, err := sessionkit.NewConfig(cfg.Session)
	if err != nil {
		return err
	}
	v.cookies = sessionkit.NewCookies(sessionCfg)
	return nil
}

// configureJWT sets up token verification for the configured signature mode.
// An empty mode means the application authenticates with API keys only.
func (v *verifier) configureJWT(cfg *appconfig.Auth) error {
	switch cfg.Mode {
	case "":
		return nil
	case appconfig.AuthModeHS256:
		if cfg.Secret == "" {
			return fmt.Errorf("authkit: %s mode needs AUTH_SECRET", appconfig.AuthModeHS256)
		}
		secret := []byte(cfg.Secret)
		v.jwtKeyfunc = func(*jwt.Token) (any, error) { return secret, nil }
		v.parserOpts = parserOptions(cfg, "HS256")
		return nil
	case appconfig.AuthModeJWKS:
		if cfg.JWKSURL == "" {
			return fmt.Errorf("authkit: %s mode needs AUTH_JWKS_URL", appconfig.AuthModeJWKS)
		}
		k, err := jwksKeyfunc(cfg)
		if err != nil {
			return err
		}
		v.jwtKeyfunc = k
		v.parserOpts = parserOptions(cfg, "RS256")
		return nil
	default:
		return fmt.Errorf("authkit: unknown AUTH_MODE %q: must be %q or %q",
			cfg.Mode, appconfig.AuthModeHS256, appconfig.AuthModeJWKS)
	}
}

// parserOptions builds the claim checks. Pinning the signing method is what
// stops an algorithm confusion attack, where a token signed with a public key is
// replayed against a shared-secret verifier.
func parserOptions(cfg *appconfig.Auth, method string) []jwt.ParserOption {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{method}),
		jwt.WithExpirationRequired(),
	}
	// An empty issuer or audience means the deployment does not pin one.
	if cfg.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(cfg.Issuer))
	}
	if cfg.Audience != "" {
		opts = append(opts, jwt.WithAudience(cfg.Audience))
	}
	return opts
}

// jwksKeyfunc fetches the issuer's public keys and keeps them refreshed, which
// is what makes key rotation take effect without a restart.
func jwksKeyfunc(cfg *appconfig.Auth) (jwt.Keyfunc, error) {
	storage, err := jwkset.NewStorageFromHTTP(cfg.JWKSURL, jwkset.HTTPClientStorageOptions{
		Ctx:             context.Background(),
		RefreshInterval: cfg.JWKSCacheTTL,
		RefreshErrorHandler: func(_ context.Context, err error) {
			loggear.Error("refreshing the JWKS failed; continuing with the cached keys",
				"url", cfg.JWKSURL, "error", err)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("authkit: reading the key set from %s: %w", cfg.JWKSURL, err)
	}

	k, err := keyfunc.New(keyfunc.Options{Ctx: context.Background(), Storage: storage})
	if err != nil {
		return nil, fmt.Errorf("authkit: building the key function: %w", err)
	}
	return k.Keyfunc, nil
}

// Verify reads the credentials on a request, in the order they are trusted.
//
// ⚠ A header beats a cookie, always. A browser attaches a cookie to every
// request by itself, while an Authorization header or an API key was put there
// on purpose — so letting the cookie win would let whatever the browser happens
// to be holding override what the caller actually asked for.
func (v *verifier) Verify(r *rest.Request) (*VerifyResult, error) {
	if token, ok := bearerToken(r); ok {
		principal, err := v.verifyJWT(token)
		if err != nil {
			return nil, err
		}
		return &VerifyResult{Principal: principal}, nil
	}
	if key, ok := r.Header(v.apiKeyHeader); ok && key != "" {
		principal, err := v.verifyAPIKey(r.Context(), key)
		if err != nil {
			return nil, err
		}
		return &VerifyResult{Principal: principal}, nil
	}
	return v.verifySession(r)
}

// verifySession resolves the session cookie, if there is one.
//
// ⚠ An unusable cookie is not a refusal. A browser sends whatever it is holding
// without anyone deciding to, so a session that has expired or been signed out
// elsewhere means the request is simply anonymous — and the browser is told to
// drop the cookie so it stops presenting a dead one for weeks. An endpoint that
// needs a caller still answers 401 on its own declaration.
//
// That is deliberately unlike a bad token or a bad API key, which end the
// request at 401 even on a public endpoint. The asymmetry mirrors the real
// difference between a credential someone chose to send and one the browser
// sent on its own.
func (v *verifier) verifySession(r *rest.Request) (*VerifyResult, error) {
	if v.sessions == nil {
		return &VerifyResult{}, nil
	}
	id, ok := r.Cookie(v.cookies.Name())
	if !ok || id == "" {
		return &VerifyResult{}, nil
	}

	session, err := v.sessions.Lookup(r.Context(), id)
	switch {
	case err == nil:
		return &VerifyResult{Principal: PrincipalOf(session.Snapshot)}, nil

	case errors.Is(err, sessionkit.ErrSessionNotFound):
		// The store answered, and the session is gone. Take the cookie back.
		loggear.Info("a request presented a session cookie that no longer resolves",
			"type", LogTypeSessionRejected,
			"cookie", v.cookies.Name(),
			"method", r.Method(),
			"path", r.Path())
		return &VerifyResult{Discard: []*http.Cookie{v.cookies.Discard()}}, nil

	default:
		// ⚠ No discard here, and that is the point of the whole distinction.
		// Telling every browser to drop its cookie because the store is
		// unreachable would sign out every user at once, and they would not come
		// back when it recovered.
		loggear.Error("the session store could not be asked about a session cookie",
			"type", LogTypeSessionStoreUnavailable,
			"cookie", v.cookies.Name(),
			"method", r.Method(),
			"path", r.Path(),
			"error", err)
		return nil, fmt.Errorf("%w: %w", ErrCredentialUnavailable, err)
	}
}

// bearerToken pulls the token out of the Authorization header.
// A header with another scheme is reported as a credential we cannot use,
// rather than as no credential at all.
func bearerToken(r *rest.Request) (string, bool) {
	value, ok := r.Header(authorizationHeader)
	if !ok || value == "" {
		return "", false
	}
	token, found := strings.CutPrefix(value, bearerPrefix)
	if !found {
		return "", true // present but not a bearer token: verifyJWT rejects it
	}
	return strings.TrimSpace(token), true
}

func (v *verifier) verifyJWT(token string) (*Principal, error) {
	if v.jwtKeyfunc == nil {
		return nil, fmt.Errorf("%w: this application does not accept tokens", ErrInvalidCredential)
	}
	if token == "" {
		return nil, fmt.Errorf("%w: the Authorization header is not a bearer token", ErrInvalidCredential)
	}

	parsed, err := jwt.Parse(token, v.jwtKeyfunc, v.parserOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredential, err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("%w: unexpected claims shape", ErrInvalidCredential)
	}
	subject, _ := claims.GetSubject()

	return &Principal{
		Subject: subject,
		Scopes:  scopesOf(claims),
		Scheme:  SchemeJWT,
		Claims:  claims,
	}, nil
}

// scopesOf reads the space-separated scope claim (RFC 8693).
func scopesOf(claims jwt.MapClaims) []string {
	scope, ok := claims[scopeClaim].(string)
	if !ok {
		return nil
	}
	return strings.Fields(scope)
}

// Schemes reports what this verifier was built to check.
func (v *verifier) Schemes() []string {
	var schemes []string
	if v.jwtKeyfunc != nil {
		schemes = append(schemes, SchemeJWT)
	}
	if v.keys != nil {
		schemes = append(schemes, SchemeAPIKey)
	}
	if v.sessions != nil {
		schemes = append(schemes, SchemeSession)
	}
	return schemes
}

func (v *verifier) verifyAPIKey(ctx context.Context, key string) (*Principal, error) {
	if v.keys == nil {
		return nil, fmt.Errorf("%w: this application does not accept API keys", ErrInvalidCredential)
	}
	principal, err := v.keys.Lookup(ctx, key)
	switch {
	case err == nil:
		return principal, nil
	case errors.Is(err, ErrKeyNotFound):
		// The store answered, and the answer is no.
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredential, err)
	default:
		// The store did not answer. Nothing is known about this key, so
		// nothing may be concluded about it.
		return nil, fmt.Errorf("%w: %w", ErrCredentialUnavailable, err)
	}
}

// staticKeyStore accepts the keys listed in the configuration. It is the default
// so that a project can use API keys without writing any storage code; projects
// that issue and revoke keys at runtime inject their own KeyStore instead.
func staticKeyStore(keys []string) KeyStore {
	accepted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		accepted[key] = struct{}{}
	}

	return KeyStoreFunc(func(_ context.Context, key string) (*Principal, error) {
		if _, ok := accepted[key]; !ok {
			return nil, ErrKeyNotFound
		}
		// The configured keys carry no owner or permissions of their own.
		return &Principal{Subject: apiKeySubjectPrefix, Scheme: SchemeAPIKey}, nil
	})
}
