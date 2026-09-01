// Package middleware holds the middleware that runs for the whole application.
//
// Unlike the plamo packages, this is application code: a project is expected to
// read it and adjust it. The authentication middleware below decides *when* a
// request is refused; the credential verification itself lives in plamo/authkit
// and the shape of the refusal in plamo/restkit, so that a change here cannot
// weaken how credentials are checked or make one error look unlike the others.
package middleware

import (
	"errors"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// Auth verifies the credential on each request and records the caller on the
// request context, where endpoints and application code read it with
// authkit.PrincipalFrom.
//
// It does not decide whether an endpoint needs a caller: that is the endpoint's
// own declaration (Endpoint.Security), enforced in restkit. A request with no
// credential passes through untouched, so a public endpoint is still served.
func Auth(verifier authkit.Verifier) rest.Middleware {
	return func(next rest.Handler) rest.Handler {
		return func(r *rest.Request) *rest.Response {
			if res := authenticate(verifier, r); res != nil {
				return res
			}
			return next(r)
		}
	}
}

// AuthUpgrade is the same check placed in front of a WebSocket upgrade.
//
// The WebSocket layer takes a pre-upgrade handler rather than a wrapping
// middleware, and treats a non-nil response as "refuse the upgrade". Rejecting
// here means an unusable credential never opens a connection at all.
func AuthUpgrade(verifier authkit.Verifier) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		return authenticate(verifier, r)
	}
}

// authenticate verifies the request's credential and puts the caller on its
// context. It returns a response only when the request must be refused outright,
// and nil when it should continue.
//
// This is the policy a project may want to change — accepting a credential from
// somewhere other than a header, or refusing anonymous requests at the edge.
func authenticate(verifier authkit.Verifier, r *rest.Request) *rest.Response {
	principal, err := verifier.Verify(r)
	switch {
	case err == nil:
		r.SetContext(authkit.WithPrincipal(r.Context(), principal))
		return nil
	case errors.Is(err, authkit.ErrNoCredential):
		// No credential is not a failure here. Public endpoints are served
		// without one; endpoints that need a caller answer 401 themselves.
		return nil
	case errors.Is(err, authkit.ErrCredentialUnavailable):
		// Nothing was decided about the credential, because whatever holds the
		// answer did not respond. Answering 401 here would tell every caller in
		// the system that their credential is bad at the moment nothing can
		// check any of them — and a browser would act on that by signing out.
		//
		// The request is refused rather than served anonymously: the caller
		// asked to be identified, and serving them as nobody would quietly
		// downgrade what they can do instead of failing.
		return restkit.UnavailableResponse()
	default:
		// A credential was presented and cannot be trusted. Refuse it even for a
		// public endpoint: accepting it silently would hide the caller's bug.
		return restkit.UnauthenticatedResponse()
	}
}
