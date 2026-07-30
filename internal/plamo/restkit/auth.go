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

// ForbiddenCode is the machine-readable code for a caller the application knows
// but that may not perform this operation.
const ForbiddenCode = "forbidden"

// forbiddenMessage is what such a caller is told.
const forbiddenMessage = "this operation is not allowed for the caller"

// challengeHeader and challengeScheme tell a rejected caller how to authenticate
// (RFC 6750).
const (
	challengeHeader = "WWW-Authenticate"
	challengeScheme = "Bearer"
)

// ForbiddenResponse is the answer given to a caller the application recognises
// but that lacks what this operation requires.
//
// It is deliberately separate from UnauthenticatedResponse: 401 tells a caller
// to say who they are, 403 tells them that doing so again will not help.
func ForbiddenResponse() *rest.Response {
	return &rest.Response{
		Code: http.StatusForbidden,
		Body: &ErrorEnvelope{Error: ErrorDetail{
			Code:    ForbiddenCode,
			Message: forbiddenMessage,
		}},
	}
}

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
