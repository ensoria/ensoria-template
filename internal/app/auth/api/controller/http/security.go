// Package http serves the two endpoints that turn a verified token into a
// browser session and end it again.
//
// # Why a session is traded for a token rather than issued here
//
// Nothing in this package decides who anyone is. The caller arrives with a
// token an identity provider signed, the ordinary verifier checks it, and what
// these endpoints add is a credential the server can take back: a random id in
// an HttpOnly cookie, matched against a record. Signing out — or revoking every
// session a subject holds — then takes effect on the next request, rather than
// whenever the token would have expired.
//
// # Why the browser gets a cookie instead of the token
//
// A token a browser has to store is a token script can read. In localStorage it
// is readable by anything injected into the page; in a readable cookie the same
// is true. An HttpOnly cookie is the one place a browser can keep a credential
// that its own JavaScript cannot reach. The cost is that browsers attach it to
// cross-site requests too, which is what the cross-origin check in
// internal/middleware exists to answer.
package http

// Error codes returned by the session endpoints.
//
// There is deliberately no code for "the token is invalid" or "you are not
// signed in". Those are answered before a handler runs — by the authentication
// middleware and by the endpoint's own security declaration — so that every
// endpoint in the application refuses a caller the same way.
const (
	// CodeSessionNotCreated is returned when a verified caller could not be
	// given a session, because the store did not answer.
	CodeSessionNotCreated = "session_not_created"
	// CodeSessionNotEnded is returned when a live session could not be revoked,
	// for the same reason.
	//
	// ⚠ It is an error rather than a shrug on purpose. Answering 204 to a
	// logout that did not happen tells the browser to drop its cookie while the
	// session stays valid on the server — and the session id it just dropped is
	// the only thing that could have been used to end it.
	CodeSessionNotEnded = "session_not_ended"
)

// The labels on the records these endpoints write. They are constants so that a
// log platform can be given an alert condition that survives a change of
// wording, and so that a test and that condition read the same value.
const (
	// LogTypeSessionNotCreated marks a caller who presented a good token and
	// was not given a session. It means the store is not answering, and every
	// sign-in is failing for as long as that is true.
	LogTypeSessionNotCreated = "session_not_created_log"
	// LogTypeSessionNotEnded marks a logout that did not take effect. The
	// session is still valid and the caller believes they are signed out.
	LogTypeSessionNotEnded = "session_not_ended_log"
	// LogTypeReplacedSessionKept marks a session left behind by a new sign-in
	// from the same browser. It is the mildest of the three: the record is
	// unreachable from that browser and expires on its own.
	LogTypeReplacedSessionKept = "replaced_session_kept_log"
)
