package wskit

import "github.com/ensoria/ensoria-template/internal/plamo/authkit"

// SecuritySpec declares who may open a connection to a channel.
//
// It reads the way restkit's declaration for an endpoint does, because a
// WebSocket channel is an entry point from outside just as an endpoint is:
//
//   - nil requires a verified caller. A channel whose author never thought
//     about access ends up closed rather than open to anyone.
//   - Public opens the channel to everyone, anonymous callers included. It is
//     the only way to open one, and it has to be written down.
//   - Schemes limits which kinds of credential are accepted; empty accepts any
//     the application verifies.
//   - Scopes are the permissions the caller must hold — all of them.
//
// The declaration is checked before the upgrade, while the handshake is still
// an HTTP request, so a refused caller gets an HTTP 401 or 403 and no
// connection is ever opened.
//
// There is no resource rule here. Whether a caller may join this particular
// room is decided by the channel itself — in a pre-upgrade middleware or in a
// handler — where the room is known.
//
// ⚠ Public states no condition: a public channel admits everyone before
// Schemes and Scopes are looked at, so the startup checks refuse a declaration
// carrying Public with either.
type SecuritySpec struct {
	Public  bool
	Schemes []string
	Scopes  []string
}

// Decision is what a declaration says about a caller.
type Decision int

const (
	// Allowed lets the caller through.
	Allowed Decision = iota
	// Unauthenticated refuses because a verified caller is required and there
	// is none. Over HTTP it is a 401: saying who you are may help.
	Unauthenticated
	// Forbidden refuses a caller who is known but lacks an accepted kind of
	// credential or a required scope. Over HTTP it is a 403: saying it again
	// will not help.
	Forbidden
)

// Decide judges caller — nil for an anonymous one — against the declaration.
//
// Scopes are judged through caller.SatisfiesScopes, so the application's scope
// policy counts when the caller carries it.
func (s *SecuritySpec) Decide(caller *authkit.Principal) Decision {
	if s != nil && s.Public {
		return Allowed
	}
	if caller == nil {
		return Unauthenticated
	}
	var schemes, scopes []string
	if s != nil {
		schemes, scopes = s.Schemes, s.Scopes
	}
	if !caller.HasScheme(schemes) || !caller.SatisfiesScopes(scopes) {
		return Forbidden
	}
	return Allowed
}

// RequiresCaller reports whether the declaration admits only verified callers.
// The startup checks use it to refuse a channel that needs a caller in an
// application that can verify none.
func (s *SecuritySpec) RequiresCaller() bool {
	return s == nil || !s.Public
}

// Contradictory reports whether the declaration says Public and also states a
// condition, which a public channel never checks.
func (s *SecuritySpec) Contradictory() bool {
	return s != nil && s.Public && (len(s.Schemes) > 0 || len(s.Scopes) > 0)
}

// MessageSecurity adds conditions for sending one message, on top of what the
// channel requires.
//
// It can only add. A connection exists only once the channel's conditions were
// met, so there is nothing for a message to relax, and the reading follows:
//
//   - nil adds nothing: the channel's declaration is all there is.
//   - Any declaration requires a verified caller, even on a public channel.
//     &wskit.MessageSecurity{} reads "signed in".
//   - Scopes are the permissions the caller must also hold — all of them.
//
// That lets one public channel be read by anyone and written to only by some.
//
// ⚠ nil means the opposite of what it means on Channel.Security, where it
// requires a caller. The difference is deliberate: a channel is an entry point,
// and forgetting its declaration must close it; a message's declaration is a
// condition added to that, and forgetting it adds none.
//
// There are no Schemes: a connection carries the one credential it was opened
// with, so which kinds are accepted is the channel's to decide.
//
// The message is judged before its payload is decoded, so a caller who may not
// send it never learns its rules from a validation failure. A refusal is an
// error message (unauthenticated or forbidden), and the connection stays open.
type MessageSecurity struct {
	Scopes []string
}

// Decide judges caller — nil for an anonymous one — against the declaration.
//
// Scopes are judged through caller.SatisfiesScopes, so the application's scope
// policy counts when the caller carries it.
func (s *MessageSecurity) Decide(caller *authkit.Principal) Decision {
	if s == nil {
		return Allowed
	}
	if caller == nil {
		return Unauthenticated
	}
	if !caller.SatisfiesScopes(s.Scopes) {
		return Forbidden
	}
	return Allowed
}

// RequiresCaller reports whether the declaration admits only verified callers,
// which every declaration does. The startup checks use it to refuse a message
// that needs a caller in an application that can verify none.
func (s *MessageSecurity) RequiresCaller() bool {
	return s != nil
}
