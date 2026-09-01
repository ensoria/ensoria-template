// Package sessionkit keeps browser sessions: it creates them, resolves them,
// ends them, and writes the cookie that carries one.
//
// # Why sessions exist here at all
//
// A browser needs a credential it can hold between requests. Neither of the
// other two will do: a JWT put where JavaScript can read it is readable by
// anything injected into the page, and an API key is a machine's credential.
// What a browser gets instead is an opaque id in an HttpOnly cookie, matched
// against a record on the server.
//
// Nothing here issues an identity. A session is traded for a JWT that was
// already verified, so the identity provider is still the only thing that says
// who anyone is. What the trade buys is a credential the server can take back:
// signing out, or ending every session a subject holds, takes effect on the
// next request rather than whenever a token would have expired.
//
// # The line between this package and authkit
//
// authkit verifies credentials; it does not manage their lifetime. So the
// verifier reads a session id and asks for the caller behind it, and everything
// else — creating, ending, deadlines, cookie attributes — is here.
//
// The dependency runs one way, authkit → sessionkit, which is why this package
// stores a Snapshot rather than an authkit.Principal. That is not only to break
// a cycle: a Principal carries the scheme the caller authenticated with, and
// that value must not survive the trade. The snapshot is taken from a caller
// who presented a JWT, and every request that later restores it presents a
// cookie. Storing the field would mean remembering to overwrite it on the way
// out, every time, forever.
package sessionkit

import (
	"context"
	"errors"
	"time"
)

// ErrSessionNotFound reports that no live session has that id: it was never
// created, it was ended, or it has expired.
//
// ⚠ It is the only benign outcome a Store may report, and every other error
// means the store could not be asked. Callers act on the difference — a session
// that is genuinely gone tells the browser to stop sending the cookie, while a
// store that cannot be reached must not, because doing so during an outage
// signs out every user at once and they do not come back when it ends.
//
// A Store implementation must therefore never report an outage as this error.
// The reverse mistake is safe by construction: an unrecognized error is treated
// as "could not ask", which is the cautious reading.
var ErrSessionNotFound = errors.New("sessionkit: no such session")

// Snapshot is who the caller was at the moment the session was created.
//
// It is a copy, not a reference: nothing re-reads the token afterwards, which
// is what lets a session outlive the short-lived JWT it was traded for. The
// consequence is that a change of permissions reaches a session only when a new
// one is created — which is the same rule as for a JWT, whose claims are fixed
// when it is signed.
//
// ⚠ The fields are the wire shape of "a verified caller, serialized", and the
// JSON tags are load-bearing: records outlive a deployment, so renaming a tag
// signs out everyone holding a session written by the previous version. Carrying
// a caller between services — over a message broker, say — wants the same shape,
// so treat it as a format rather than as this package's private business.
//
// ⚠ Claims comes back from storage in JSON's type system, not Go's: every
// number is a float64 and every array a []any, however it was written. So
// claims["level"].(int) fails on a value that was written as an int, and it
// fails the same way for a caller who presented a JWT — JSON is what both went
// through. Read them with care until typed accessors exist.
type Snapshot struct {
	Subject string         `json:"sub"`
	Scopes  []string       `json:"scopes,omitempty"`
	Claims  map[string]any `json:"claims,omitempty"`
}

// Clone returns a copy that shares nothing with the receiver, so that a caller
// holding a session cannot reach into the stored record.
//
// Claims is copied one level deep. A claim whose value is itself a map or a
// slice is still shared, which is the same depth encoding/json produces and as
// far as a copy can go without knowing the shapes.
func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}

	clone := &Snapshot{Subject: s.Subject}
	if s.Scopes != nil {
		clone.Scopes = append([]string(nil), s.Scopes...)
	}
	if s.Claims != nil {
		clone.Claims = make(map[string]any, len(s.Claims))
		for key, value := range s.Claims {
			clone.Claims[key] = value
		}
	}
	return clone
}

// Session is one browser session: who it belongs to, and the two deadlines it
// lives under.
//
// A session ends at whichever deadline comes first. ExpiresAt is fixed when the
// session is created and is what a stolen cookie cannot outlive; the idle
// deadline is LastSeenAt plus the configured idle limit, and moves forward as
// the session is used, which is what reclaims one nobody came back to.
type Session struct {
	// ID is the opaque value the cookie carries. It identifies the session and
	// nothing else: it is random, so it says nothing about who holds it.
	ID string `json:"id"`
	// Snapshot is the caller the session was created for.
	Snapshot *Snapshot `json:"snapshot"`
	// Persistent records which lifetime profile was chosen, which decides both
	// ExpiresAt and whether the cookie survives closing the browser.
	Persistent bool `json:"persistent"`

	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// Store keeps sessions. It is an interface so that a project can put them
// somewhere this framework has never heard of; NewStore is the implementation
// that ships.
//
// ⚠ Every method reports "no such session" as ErrSessionNotFound and everything
// else as an error meaning the store could not be asked. See ErrSessionNotFound
// for why the difference decides whether a user stays signed in through an
// outage.
type Store interface {
	// Create records a new session for the caller in snapshot and returns it.
	//
	// The id is always new. Reusing one — keeping the id a caller already had
	// and rewriting what it points at — is what session fixation needs: an
	// attacker plants an id in the victim's browser and waits for it to become
	// theirs. A fresh id makes the planted one worthless.
	//
	// persistent selects the lifetime profile ("keep me signed in").
	Create(ctx context.Context, snapshot *Snapshot, persistent bool) (*Session, error)

	// Lookup returns the live session with that id, moving its idle deadline
	// forward. ErrSessionNotFound when there is none.
	//
	// ⚠ It writes. Resolving a session is what proves it is still in use, so
	// the read path is also what keeps it alive; a store put behind a read-only
	// replica would expire every session that is actually being used.
	Lookup(ctx context.Context, id string) (*Session, error)

	// Revoke ends one session. Ending one that is already gone is not an error:
	// signing out is idempotent, and a caller signing out twice has got what
	// they asked for both times.
	Revoke(ctx context.Context, id string) error

	// RevokeSubject ends every session a subject holds, including ones this
	// process has never seen. It is what a password reset, a compromised
	// account, or an administrator disabling a user has to reach.
	//
	// Sessions created after the call are unaffected, so signing in again works
	// immediately.
	//
	// # Exposing this
	//
	// No endpoint in this template calls it: who may end another person's
	// sessions is an application's own policy, and a framework that shipped the
	// endpoint would be deciding it. To add one:
	//
	//   - Declare it like any other endpoint, with a scope only administrators
	//     hold (Endpoint.Security). An endpoint that ends anyone's sessions and
	//     forgets to say who may call it is the worst kind of endpoint to get
	//     wrong.
	//   - Take the subject from the request body or path, never from the
	//     caller's own principal — those are two different people.
	//   - Answer the same way whether or not the subject had any sessions.
	//     Reporting the count turns the endpoint into a way to ask who is
	//     signed in.
	RevokeSubject(ctx context.Context, subject string) error
}
