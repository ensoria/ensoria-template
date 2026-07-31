package authkit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/golang-jwt/jwt/v5"
)

// ErrNoCredential means the request carried no credential at all. That is not a
// failure by itself: a public endpoint is served without one, so the caller of
// Verify decides what it means.
//
// ErrInvalidCredential means a credential was presented and could not be
// trusted. That always ends the request with 401, even on a public endpoint,
// because silently ignoring a broken credential hides bugs from the caller.
var (
	ErrNoCredential      = errors.New("no credential presented")
	ErrInvalidCredential = errors.New("credential could not be verified")
)

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

// Verifier turns the credentials on a request into a Principal.
type Verifier interface {
	// Verify returns the caller the request authenticated as.
	// It returns ErrNoCredential when the request carried none, and an error
	// wrapping ErrInvalidCredential when it carried one that cannot be trusted.
	Verify(r *rest.Request) (*Principal, error)
	// Schemes reports which kinds of credential this verifier can check, in a
	// stable order (SchemeJWT, SchemeAPIKey).
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
type KeyStore interface {
	Lookup(key string) (*Principal, error)
}

// KeyStoreFunc adapts a plain function to KeyStore.
type KeyStoreFunc func(key string) (*Principal, error)

func (f KeyStoreFunc) Lookup(key string) (*Principal, error) { return f(key) }

// verifier verifies a JWT, an API key, or both, depending on what is configured.
type verifier struct {
	jwtKeyfunc   jwt.Keyfunc
	parserOpts   []jwt.ParserOption
	apiKeyHeader string
	keys         KeyStore
}

// NewVerifier builds the verifier described by the configuration.
//
// keys overrides where API keys are looked up; pass nil to use the keys from the
// configuration. Returns an error when the configuration cannot be satisfied
// (a shared-secret setup with no secret, for instance), so that a
// misconfiguration is caught at startup rather than on the first request.
func NewVerifier(cfg *appconfig.Auth, keys KeyStore) (Verifier, error) {
	if cfg == nil {
		return nil, errors.New("authkit: no auth configuration")
	}

	v := &verifier{apiKeyHeader: cfg.APIKeyHeader, keys: keys}
	if v.apiKeyHeader == "" {
		v.apiKeyHeader = appconfig.DefaultAPIKeyHeader
	}
	if v.keys == nil && len(cfg.APIKeys) > 0 {
		v.keys = staticKeyStore(cfg.APIKeys)
	}

	if err := v.configureJWT(cfg); err != nil {
		return nil, err
	}
	return v, nil
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

func (v *verifier) Verify(r *rest.Request) (*Principal, error) {
	if token, ok := bearerToken(r); ok {
		return v.verifyJWT(token)
	}
	if key, ok := r.Header(v.apiKeyHeader); ok && key != "" {
		return v.verifyAPIKey(key)
	}
	return nil, ErrNoCredential
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
	return schemes
}

func (v *verifier) verifyAPIKey(key string) (*Principal, error) {
	if v.keys == nil {
		return nil, fmt.Errorf("%w: this application does not accept API keys", ErrInvalidCredential)
	}
	principal, err := v.keys.Lookup(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredential, err)
	}
	return principal, nil
}

// staticKeyStore accepts the keys listed in the configuration. It is the default
// so that a project can use API keys without writing any storage code; projects
// that issue and revoke keys at runtime inject their own KeyStore instead.
func staticKeyStore(keys []string) KeyStore {
	accepted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		accepted[key] = struct{}{}
	}

	return KeyStoreFunc(func(key string) (*Principal, error) {
		if _, ok := accepted[key]; !ok {
			return nil, errors.New("unknown API key")
		}
		// The configured keys carry no owner or permissions of their own.
		return &Principal{Subject: apiKeySubjectPrefix, Scheme: SchemeAPIKey}, nil
	})
}
