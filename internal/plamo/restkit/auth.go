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

// UnavailableCode is the machine-readable code for a request that could not be
// judged, because something this application depends on did not answer.
const UnavailableCode = "unavailable"

// unavailableMessage says only that the failure is on this side and that trying
// again is worthwhile. What exactly is down is not the caller's business, and
// naming it tells a prober which dependency to attack.
const unavailableMessage = "the request could not be completed; try again shortly"

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

// UnavailableResponse is the answer given when the application could not decide
// whether to serve the request, because whatever holds the answer — a key
// store, a session store — could not be reached.
//
// ⚠ It is deliberately not a 401. A store that is down says nothing about the
// caller's credential, and answering 401 during an outage tells every caller in
// the system that their perfectly good credential is bad. Worse, a browser
// takes that as a reason to sign out. 503 says what is true: this side failed,
// and the same request may well succeed later.
func UnavailableResponse() *rest.Response {
	return &rest.Response{
		Code: http.StatusServiceUnavailable,
		Body: &ErrorEnvelope{Error: ErrorDetail{
			Code:    UnavailableCode,
			Message: unavailableMessage,
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
