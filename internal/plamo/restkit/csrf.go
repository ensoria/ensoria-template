package restkit

import (
	"net/http"

	"github.com/ensoria/rest/pkg/rest"
)

// CrossOriginCode is the machine-readable code for a state-changing request a
// browser sent from an origin this application does not trust.
const CrossOriginCode = "cross_origin_denied"

// crossOriginMessage is what such a caller is told.
//
// It names the origin as the problem rather than saying "forbidden", because
// the caller who sees it is usually a developer whose frontend is served from a
// port nobody added to CORS_ALLOW_ORIGIN — and the alternative reading, that
// their credential is insufficient, sends them to look in the wrong place.
const crossOriginMessage = "this request did not come from an allowed origin"

// CrossOriginResponse is the answer given to a request the cross-origin check
// refused.
//
// ⚠ It is 403 rather than 401, and the difference is not cosmetic. The request
// was refused because of where it came from, not because of who sent it:
// presenting a credential — or a better one — changes nothing. A 401 would
// invite a browser to try again, and the request that came back would be
// refused for the same reason.
func CrossOriginResponse() *rest.Response {
	return &rest.Response{
		Code: http.StatusForbidden,
		Body: &ErrorEnvelope{Error: ErrorDetail{
			Code:    CrossOriginCode,
			Message: crossOriginMessage,
		}},
	}
}
