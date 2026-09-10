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

	// implied is the application's scope policy: what Scopes stand for. It is
	// attached once the caller has been verified (see WithScopeExpander) and is
	// what makes a caller holding admin satisfy an endpoint asking for
	// orders:write.
	//
	// It is deliberately not part of Scopes. Scopes is what the credential
	// carries, and it is what a session stores when a caller trades a token for
	// one — expanding it in place would freeze today's policy into every
	// session, so tomorrow's edit to the table would not reach the browsers
	// already signed in.
	//
	// ⚠ Being unexported, it is not carried over by code that copies a
	// Principal field by field (a KeyStore building one, SnapshotOf, a
	// verifier restoring a session). That is harmless because every one of
	// those runs before the policy is attached — but it does mean that
	// anything rebuilding a Principal later, such as a periodic re-check of a
	// live connection, has to attach it again afterwards.
	implied ScopeExpander
}

// WithScopeExpander returns a copy of the caller whose scope checks also count
// what the held scopes imply. A nil expander returns the caller unchanged,
// which is what an application that states no policy gets.
//
// It returns a copy rather than writing to the receiver because a KeyStore may
// well answer two requests with the same *Principal: attaching in place would
// let one request's policy follow a value that another request is reading.
func (p *Principal) WithScopeExpander(e ScopeExpander) *Principal {
	if p == nil || e == nil {
		return p
	}
	attached := *p
	attached.implied = e
	return &attached
}

// EffectiveScopes returns every scope the caller effectively holds: what the
// credential carries, followed by what the policy adds to it.
//
// It exists so that a decision can be looked at rather than only made. "This
// caller was refused" is the hard thing to debug about implications, and the
// answer is nearly always in the difference between these two halves.
func (p *Principal) EffectiveScopes() []string {
	if p == nil {
		return nil
	}
	if p.implied == nil {
		return slices.Clone(p.Scopes)
	}
	return p.implied.Expand(p.Scopes)
}

// HasScopes reports whether the caller holds every required scope.
//
// The check is AND, not OR: OpenAPI reads `security: [{scheme: [a, b]}]` as
// "needs both", so requiring all of them keeps the generated document and the
// running code saying the same thing.
//
// The requirement is compared against EffectiveScopes, so an attached policy
// applies here and nowhere else. That is the whole reason the policy hangs off
// the Principal instead of being passed to each decision: this is the only
// place scopes are judged, so there is no second way to judge them that could
// forget to expand.
func (p *Principal) HasScopes(required []string) bool {
	if p == nil {
		return false
	}
	if len(required) == 0 {
		return true
	}

	held := p.Scopes
	if p.implied != nil {
		held = p.implied.Expand(p.Scopes)
	}
	for _, scope := range required {
		if !slices.Contains(held, scope) {
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
