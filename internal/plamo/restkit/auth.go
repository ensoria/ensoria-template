package restkit

import (
	"errors"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/rest/pkg/rest"
)

// UnauthenticatedCode is the machine-readable code for a request that could not
// be attributed to a caller.
const UnauthenticatedCode = "unauthenticated"

// unauthenticatedMessage is what the caller is told. It deliberately says
// nothing about why verification failed, so that a probe learns nothing.
const unauthenticatedMessage = "authentication is required"

// challengeHeader and challengeScheme tell a rejected caller how to authenticate
// (RFC 6750).
const (
	challengeHeader = "WWW-Authenticate"
	challengeScheme = "Bearer"
)

// AuthMiddleware verifies the credential on each request and records the caller
// on the request context, where restkit endpoints and application code read it
// with authkit.PrincipalFrom.
//
// It does not decide whether an endpoint needs a caller: that is the endpoint's
// own declaration (Endpoint.Security). A request with no credential passes
// through untouched, so a public endpoint is still served.
func AuthMiddleware(verifier authkit.Verifier) rest.Middleware {
	return func(next rest.Handler) rest.Handler {
		return func(r *rest.Request) *rest.Response {
			if res := authenticate(verifier, r); res != nil {
				return res
			}
			return next(r)
		}
	}
}

// AuthUpgradeGuard is the same check placed in front of a WebSocket upgrade.
//
// The WebSocket layer takes a pre-upgrade handler rather than a wrapping
// middleware, and treats a non-nil response as "refuse the upgrade". Rejecting
// here means an unusable credential never opens a connection at all.
func AuthUpgradeGuard(verifier authkit.Verifier) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		return authenticate(verifier, r)
	}
}

// authenticate verifies the request's credential and puts the caller on its
// context. It returns a response only when the request must be refused outright,
// and nil when it should continue.
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
	default:
		// A credential was presented and cannot be trusted. Refuse it even for a
		// public endpoint: accepting it silently would hide the caller's bug.
		return unauthenticatedResponse()
	}
}

func unauthenticatedResponse() *rest.Response {
	return &rest.Response{
		Code:       http.StatusUnauthorized,
		AddHeaders: map[string]string{challengeHeader: challengeScheme},
		Body: &ErrorEnvelope{Error: ErrorDetail{
			Code:    UnauthenticatedCode,
			Message: unauthenticatedMessage,
		}},
	}
}
