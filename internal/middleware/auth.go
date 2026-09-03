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
			result, refusal := authenticate(verifier, r)
			if refusal != nil {
				return refusal
			}

			res := next(r)
			// The instruction to drop a dead cookie rides on whatever the
			// handler answered, which is why it is applied here rather than
			// where the verdict was reached: verification happens before the
			// response exists.
			return withDiscardedCookies(res, result)
		}
	}
}

// withDiscardedCookies puts the verdict's discard instructions on the response.
//
// A nil response is left alone: some handlers answer with one, and there is
// nowhere to put a Set-Cookie on it. The instruction is not lost for long — the
// browser sends the same dead cookie with its next request, and that one gets
// the instruction instead.
func withDiscardedCookies(res *rest.Response, result *authkit.VerifyResult) *rest.Response {
	if res == nil || result == nil || len(result.Discard) == 0 {
		return res
	}
	res.Cookies = append(res.Cookies, result.Discard...)
	return res
}

// AuthUpgrade is the same check placed in front of a WebSocket upgrade.
//
// The WebSocket layer takes a pre-upgrade handler rather than a wrapping
// middleware, and treats a non-nil response as "refuse the upgrade". Rejecting
// here means an unusable credential never opens a connection at all.
// ⚠ An upgrade that succeeds has nowhere to put a Set-Cookie: the WebSocket
// library writes that response itself, and this handler is only consulted about
// whether to refuse. So a browser connecting to a public channel with a dead
// session cookie keeps it for now — until its next ordinary HTTP request, which
// carries the instruction back.
func AuthUpgrade(verifier authkit.Verifier) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		_, refusal := authenticate(verifier, r)
		return refusal
	}
}

// authenticate verifies the request's credential and puts the caller on its
// context.
//
// It returns the verdict and, separately, a response — non-nil only when the
// request must be refused outright. The verdict is returned even then being
// nil, because its instructions belong on whatever answer the request gets.
//
// This is the policy a project may want to change — accepting a credential from
// somewhere other than a header, or refusing anonymous requests at the edge.
func authenticate(verifier authkit.Verifier, r *rest.Request) (*authkit.VerifyResult, *rest.Response) {
	result, err := verifier.Verify(r)
	switch {
	case err == nil:
		// A result with no Principal is an ordinary anonymous request: public
		// endpoints are served without one, and endpoints that need a caller
		// answer 401 on their own declaration.
		if result.Principal != nil {
			r.SetContext(authkit.WithPrincipal(r.Context(), result.Principal))
		}
		return result, nil
	case errors.Is(err, authkit.ErrCredentialUnavailable):
		// Nothing was decided about the credential, because whatever holds the
		// answer did not respond. Answering 401 here would tell every caller in
		// the system that their credential is bad at the moment nothing can
		// check any of them — and a browser would act on that by signing out.
		//
		// The request is refused rather than served anonymously: the caller
		// asked to be identified, and serving them as nobody would quietly
		// downgrade what they can do instead of failing.
		return nil, restkit.UnavailableResponse()
	default:
		// A credential was presented and cannot be trusted. Refuse it even for a
		// public endpoint: accepting it silently would hide the caller's bug.
		return nil, restkit.UnauthenticatedResponse()
	}
}
