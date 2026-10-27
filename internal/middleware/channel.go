package middleware

import (
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"
)

// ChannelSecurity judges a WebSocket handshake against the channel's
// declaration (wskit.SecuritySpec), before the upgrade.
//
// It runs right after AuthUpgrade, which has put the caller — if there is one —
// on the request context. A refusal is an ordinary HTTP answer in the shape
// every error has: 401 when the channel needs a caller and there is none, 403
// when the caller is known and lacks an accepted kind of credential or a
// required scope. Either way no connection is opened, which is the point of
// deciding here rather than in OnOpen: a client is told why with a status it
// already knows how to handle, and nothing about the channel reaches it.
//
// A nil declaration requires a caller; only Public opens a channel.
func ChannelSecurity(security *wskit.SecuritySpec) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		caller, _ := authkit.PrincipalFrom(r.Context())
		switch security.Decide(caller) {
		case wskit.Unauthenticated:
			return restkit.UnauthenticatedResponse()
		case wskit.Forbidden:
			return restkit.ForbiddenResponse()
		default:
			return nil
		}
	}
}
