package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/auth/api/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// NewCreateSession trades a verified token for a browser session.
//
// The endpoint accepts only the token scheme. Accepting a session cookie here
// as well would let a session renew itself indefinitely without the identity
// provider ever being consulted again, which is exactly the property the
// absolute deadline exists to deny. An API key is refused for a plainer reason:
// it belongs to a service, and a service has no browser to put a cookie in.
//
// It requires no scope. The caller is asking to keep being who the token
// already says they are, and a scope check here would be asking permission for
// a permission they hold.
func NewCreateSession(sessions sessionkit.Store, cookies *sessionkit.Cookies) *restkit.Endpoint[dto.CreateSession, dto.Session] {
	return &restkit.Endpoint[dto.CreateSession, dto.Session]{
		Summary: "Exchange a token for a browser session",
		Description: "Creates a server-side session for the caller the bearer token identifies and " +
			"returns it as an HttpOnly cookie. Send the token in the Authorization header; " +
			"every later request authenticates with the cookie the browser stores from this response, " +
			"and needs no token at all.\n\n" +
			"The session's lifetime is independent of the token's: the token only has to be valid at " +
			"this moment. That is the point of the exchange — a short-lived token can keep a browser " +
			"signed in for as long as the session lasts.\n\n" +
			"⚠ Because the credential is a cookie, the browser attaches it to requests this " +
			"application did not initiate. Requests that change state are therefore refused unless " +
			"the browser reports an origin the deployment trusts (CORS_ALLOW_ORIGIN); a caller that " +
			"is not a browser sends no origin and is unaffected.",
		Task:    "sign in",
		Success: http.StatusCreated,
		Security: &restkit.SecuritySpec{
			Schemes: []string{authkit.SchemeJWT},
		},
		ResponseHeaders: []restkit.HeaderSpec{
			{
				Name: "Set-Cookie",
				Meaning: "The session cookie. It is HttpOnly, so script cannot read it; " +
					"a browser sends it automatically and a non-browser client has to store and " +
					"resend it itself.",
			},
		},
		FieldDocs: map[string]string{
			"persistent": "Whether to keep the caller signed in after the browser closes.",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{
				"creates a session record on the server",
				"ends the session the caller already held, if there was one",
			},
			// Two calls make two sessions with two ids, and the second one
			// replaces the first in the browser.
			Idempotent: new(false),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:    http.StatusServiceUnavailable,
				Code:      CodeSessionNotCreated,
				Condition: "The session store did not answer, so no session could be recorded.",
				CallerAction: "Retry after a short delay. The token was accepted, so signing in " +
					"again is not necessary — only the same request is.",
			},
		},
		Handle: func(r *rest.Request, body *dto.CreateSession) (*rest.Result[dto.Session], error) {
			if sessions == nil || cookies == nil {
				// Unreachable in a running application: the startup checks
				// refuse to serve these endpoints without a session store. It
				// is answered rather than left to panic because the endpoint is
				// also built by the document generator, which injects neither.
				return nil, restkit.NewError(
					http.StatusServiceUnavailable, CodeSessionNotCreated, restkit.UnavailableMessage)
			}

			// The security declaration above is what guarantees this: an
			// endpoint that is not public is never reached without a caller.
			principal, _ := authkit.PrincipalFrom(r.Context())

			session, err := sessions.Create(r.Context(), authkit.SnapshotOf(principal), body.Persistent)
			if err != nil {
				loggear.Error("a verified caller could not be given a session",
					"type", LogTypeSessionNotCreated,
					"subject", principal.Subject,
					"error", err)
				return nil, restkit.NewError(
					http.StatusServiceUnavailable, CodeSessionNotCreated, restkit.UnavailableMessage)
			}

			endReplacedSession(r, sessions, cookies)

			return rest.NewResult(&dto.Session{
				Subject:    session.Snapshot.Subject,
				Persistent: session.Persistent,
				ExpiresAt:  session.ExpiresAt.UTC(),
			}, rest.WithCookie(cookies.Issue(session))), nil
		},
	}
}

// endReplacedSession ends the session the browser was already holding.
//
// A browser that exchanges a token while it still has a live session leaves
// that session behind: the cookie is overwritten, so nothing can reach it from
// this browser again — but the record stays valid until its own deadline, and
// anyone who copied the old id can still use it.
//
// ⚠ It runs after the new session exists, never before. Revoking first and then
// failing to create would sign the caller out of a session that was working.
//
// A failure here is logged and no more. The caller asked for a session and has
// one; refusing the request over an old session that is already unreachable
// from their browser would trade a working sign-in for a cleanup.
func endReplacedSession(r *rest.Request, sessions sessionkit.Store, cookies *sessionkit.Cookies) {
	previous, ok := r.Cookie(cookies.Name())
	if !ok || previous == "" {
		return
	}
	if err := sessions.Revoke(r.Context(), previous); err != nil {
		loggear.Warn("the session replaced by a new sign-in could not be ended",
			"type", LogTypeReplacedSessionKept,
			"error", err)
	}
}
