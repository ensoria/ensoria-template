// Package authkit verifies the credentials a caller presents and turns them into
// a Principal that the rest of the application can read off the request context.
//
// It only verifies credentials; issuing them is left to an identity provider or
// to application code. That keeps the framework independent of any particular
// login design (user store, password policy, refresh strategy).
//
// The package lives under plamo so that a project can replace or extend it —
// swapping the API key store for a database-backed one, for example.
package authkit

import (
	"context"
	"maps"
	"slices"

	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

// The schemes a caller can authenticate with. They double as the names used in
// the generated OpenAPI security schemes.
const (
	SchemeJWT     = "jwt"
	SchemeAPIKey  = "apiKey"
	SchemeSession = "session"
)

// Principal is the verified caller: who they are and what they may do.
type Principal struct {
	// Subject identifies the caller (the `sub` claim, or the API key's owner).
	Subject string
	// Scopes are the permissions the credential carries.
	Scopes []string
	// Scheme is how the caller authenticated: SchemeJWT, SchemeAPIKey or
	// SchemeSession.
	Scheme string
	// Claims are the remaining token claims, for application code that needs
	// more than the fields above. Nil for API keys.
	Claims map[string]any
}

// HasScopes reports whether the caller holds every required scope.
//
// The check is AND, not OR: OpenAPI reads `security: [{scheme: [a, b]}]` as
// "needs both", so requiring all of them keeps the generated document and the
// running code saying the same thing.
func (p *Principal) HasScopes(required []string) bool {
	if p == nil {
		return false
	}
	for _, scope := range required {
		if !slices.Contains(p.Scopes, scope) {
			return false
		}
	}
	return true
}

// HasScheme reports whether the caller authenticated with one of the accepted
// schemes. An empty list accepts any scheme.
func (p *Principal) HasScheme(accepted []string) bool {
	if p == nil {
		return false
	}
	if len(accepted) == 0 {
		return true
	}
	return slices.Contains(accepted, p.Scheme)
}

// SnapshotOf records who a caller is, in the form a session keeps.
//
// The scheme is deliberately not carried over. A snapshot is taken from a caller
// who presented a token, and every request that later restores it presents a
// cookie — so the value would be wrong from the moment it was written. Leaving
// it out of the stored shape means it cannot be got wrong rather than having to
// be remembered.
func SnapshotOf(p *Principal) *sessionkit.Snapshot {
	if p == nil {
		return nil
	}
	return &sessionkit.Snapshot{
		Subject: p.Subject,
		Scopes:  slices.Clone(p.Scopes),
		Claims:  maps.Clone(p.Claims),
	}
}

// PrincipalOf restores the caller a session was created for.
//
// The scheme is SchemeSession whatever the caller presented when the session was
// created: this request presented a cookie, and an endpoint declaring
// Schemes: [session] is asking about this request.
func PrincipalOf(snapshot *sessionkit.Snapshot) *Principal {
	if snapshot == nil {
		return nil
	}
	return &Principal{
		Subject: snapshot.Subject,
		Scopes:  slices.Clone(snapshot.Scopes),
		Scheme:  SchemeSession,
		Claims:  maps.Clone(snapshot.Claims),
	}
}

// principalKey is unexported so that nothing outside this package can replace
// the principal on a context.
type principalKey struct{}

// WithPrincipal returns a context carrying the verified caller.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the verified caller from the context.
// ok is false when the request carried no credential.
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}
