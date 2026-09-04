package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// NewDeleteSession ends the session the caller is holding.
//
// It accepts only the session scheme, which is what makes it a logout rather
// than a way to end somebody else's session: the only session it can reach is
// the one the request itself authenticated with. Ending every session a subject
// holds is a different operation, and deliberately not an endpoint this
// template ships (see sessionkit.Store.RevokeSubject).
//
// ⚠ A caller with no live session gets 401, not 204. There is nothing to end,
// and the answer says so — but the response still carries the instruction to
// drop the cookie, added by the authentication middleware, so the browser stops
// presenting a dead session either way. A client can therefore treat 204 and
// 401 identically: both mean "you are not signed in any more".
func NewDeleteSession(sessions sessionkit.Store, cookies *sessionkit.Cookies) *restkit.Endpoint[restkit.NoBody, restkit.NoBody] {
	return &restkit.Endpoint[restkit.NoBody, restkit.NoBody]{
		Summary: "End the current session",
		Description: "Revokes the session the request authenticated with and tells the browser to " +
			"drop its cookie. The session stops working immediately and everywhere, which is what " +
			"a server-side session buys over a token that has to expire on its own.\n\n" +
			"A request with no live session is answered 401 rather than 204, and its response also " +
			"carries the instruction to drop the cookie. Both answers leave the caller signed out.",
		Task:    "sign out",
		Success: http.StatusNoContent,
		Security: &restkit.SecuritySpec{
			Schemes: []string{authkit.SchemeSession},
		},
		ResponseHeaders: []restkit.HeaderSpec{
			{
				Name:    "Set-Cookie",
				Meaning: "The instruction to drop the session cookie (the same cookie, empty, with Max-Age=0).",
			},
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"ends the session on the server"},
			// Ending a session that is already ended leaves the same state
			// behind, which is what idempotent asks.
			Idempotent: new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:    http.StatusServiceUnavailable,
				Code:      CodeSessionNotEnded,
				Condition: "The session store did not answer, so the session is still valid.",
				CallerAction: "Retry. Do not treat this as a sign-out: the session still works, " +
					"and the cookie is deliberately left in place so that retrying is possible.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[restkit.NoBody], error) {
			if sessions == nil || cookies == nil {
				// Unreachable while serving: the startup checks refuse these
				// endpoints without a store. See NewCreateSession.
				return nil, restkit.NewError(
					http.StatusServiceUnavailable, CodeSessionNotEnded, restkit.UnavailableMessage)
			}

			// The security declaration accepts only the session scheme, so the
			// request authenticated with this cookie and it is there to read.
			id, _ := r.Cookie(cookies.Name())

			if err := sessions.Revoke(r.Context(), id); err != nil {
				// ⚠ No discard instruction on this path. Telling the browser to
				// forget an id that still names a live session throws away the
				// only handle anyone had on it: the caller could no longer end
				// the session, and would believe they already had.
				loggear.Error("a session could not be ended",
					"type", LogTypeSessionNotEnded,
					"error", err)
				return nil, restkit.NewError(
					http.StatusServiceUnavailable, CodeSessionNotEnded, restkit.UnavailableMessage)
			}

			return restkit.NoContent(rest.WithCookie(cookies.Discard())), nil
		},
	}
}
