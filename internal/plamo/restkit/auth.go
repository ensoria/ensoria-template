package restkit

import (
	"net/http"

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

// UnauthenticatedResponse is the answer given to a caller the application could
// not authenticate.
//
// It lives here, next to ErrorEnvelope, so that every 401 in the application has
// the same shape as every other error. The middleware that decides *when* to
// return it lives in internal/middleware, where a project can adjust the policy.
func UnauthenticatedResponse() *rest.Response {
	return &rest.Response{
		Code:       http.StatusUnauthorized,
		AddHeaders: map[string]string{challengeHeader: challengeScheme},
		Body: &ErrorEnvelope{Error: ErrorDetail{
			Code:    UnauthenticatedCode,
			Message: unauthenticatedMessage,
		}},
	}
}
