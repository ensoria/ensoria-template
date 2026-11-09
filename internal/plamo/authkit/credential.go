package authkit

import (
	"context"
	"time"
)

// Credential is what a caller was verified with, kept so that something
// outliving the request — a WebSocket connection — can ask about it again and
// stop when it stops.
//
// What is kept depends on what can be asked. Nothing can end a token before
// its exp, so only its expiry is kept. An API key or a session can be ended by
// its store at any moment, so the key or the session's id is kept, to be looked
// up again with Verifier.Recheck.
//
// ⚠ The key and the id are secrets: whoever holds one is the caller. They are
// unexported, so that code reading a Credential off a context can neither log
// nor forward them; only the verifier reads them.
type Credential struct {
	scheme  string
	secret  string
	expires time.Time
}

// TokenCredential records a verified token by its expiry (the exp claim).
func TokenCredential(expires time.Time) *Credential {
	return &Credential{scheme: SchemeJWT, expires: expires}
}

// KeyCredential records a verified API key.
func KeyCredential(key string) *Credential {
	return &Credential{scheme: SchemeAPIKey, secret: key}
}

// SessionCredential records a verified session by its id.
func SessionCredential(id string) *Credential {
	return &Credential{scheme: SchemeSession, secret: id}
}

// Scheme is the kind of credential: SchemeJWT, SchemeAPIKey or SchemeSession.
// It is empty for no credential.
func (c *Credential) Scheme() string {
	if c == nil {
		return ""
	}
	return c.scheme
}

// Expires reports when the credential stops being valid by itself. Only a
// token does: for an API key or a session ok is false, and the store has to be
// asked instead.
//
// ⚠ It is decided by the kind of credential, never by whether the caller's
// claims hold an exp. A session keeps the claims of the token it was traded
// for, exp included, and that exp is the token's: reading it as the session's
// would end a session when a token nobody presents any more would have expired.
func (c *Credential) Expires() (time.Time, bool) {
	if c == nil || c.scheme != SchemeJWT {
		return time.Time{}, false
	}
	return c.expires, true
}

// credentialKey is unexported so that nothing outside this package can replace
// the credential on a context.
type credentialKey struct{}

// WithCredential returns a context carrying what the caller was verified with.
func WithCredential(ctx context.Context, c *Credential) context.Context {
	return context.WithValue(ctx, credentialKey{}, c)
}

// CredentialFrom returns what the caller on the context was verified with. ok
// is false when the request carried no credential.
func CredentialFrom(ctx context.Context) (*Credential, bool) {
	c, ok := ctx.Value(credentialKey{}).(*Credential)
	if !ok || c == nil {
		return nil, false
	}
	return c, true
}
