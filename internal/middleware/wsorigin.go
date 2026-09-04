package middleware

import (
	"log/slog"
	"net/url"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// LogTypeUpgradeOriginDenied labels a WebSocket upgrade refused for where it
// came from. It is separate from LogTypeCrossOriginDenied because the two mean
// different things operationally: an ordinary request refused this way is
// usually a misconfigured frontend, while a refused upgrade is the shape a
// cross-site WebSocket hijacking attempt takes.
const LogTypeUpgradeOriginDenied = "upgrade_origin_denied_log"

// UpgradeOrigin refuses a WebSocket upgrade a browser started from an origin
// this deployment does not claim.
//
// # Why the HTTP check does not cover this
//
// A WebSocket handshake is a GET, and CSRF treats GET as safe and always allows
// it — correctly, for HTTP, where a GET is not supposed to change anything. An
// upgrade is the exception: it changes a great deal, because what follows the
// GET is a two-way connection carrying the caller's session.
//
// ⚠ That is cross-site WebSocket hijacking, and it is worse than ordinary CSRF.
// The same-origin policy does not apply to WebSocket connections at all: a page
// on any site can open one to this server, the browser attaches the session
// cookie to the handshake, and — unlike a forged form post — the attacking page
// can then *read everything the connection sends*. There is no CORS to fall
// back on, so this check is the only thing standing in the way.
//
// # What is allowed
//
// A handshake with no Origin is allowed: RFC 6455 requires *browsers* to send
// one, and a caller that sends none is not a browser, so nobody's cookie was
// attached without their meaning it. A handshake whose Origin names this same
// host is allowed, which is what makes a same-origin deployment work with no
// configuration at all. Everything else has to be named in CORS_ALLOW_ORIGIN.
//
// ⚠ Origin rather than Sec-Fetch-Site, which is the reverse of what
// net/http.CrossOriginProtection prefers for ordinary requests. Every browser
// has sent Origin on a WebSocket handshake since RFC 6455, while the Sec-Fetch
// headers are not carried by every browser on this path — so here Origin is the
// header that is actually always there.
func UpgradeOrigin(origins *Origins) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		origin, ok := r.Header(originHeader)
		if !ok || origin == "" {
			// Not a browser. See above.
			return nil
		}
		if sameOrigin(r, origin) || origins.Allows(origin) {
			return nil
		}

		loggear.Warn("a WebSocket upgrade was refused for the origin it came from",
			slog.String("type", LogTypeUpgradeOriginDenied),
			slog.String("origin", origin),
			slog.String("path", r.Path()),
			slog.String("remote_addr", r.RemoteAddr()),
		)
		return restkit.CrossOriginResponse()
	}
}

// sameOrigin reports whether the origin the browser named is this same host.
//
// ⚠ The comparison is on the host alone, because Host carries no scheme: a page
// on http://example.com connecting to https://example.com reads as same-origin
// here when it is not. That is the same trade-off net/http.CrossOriginProtection
// makes, for the same reason — there is nothing in the request to compare a
// scheme against — and HSTS is what closes it, by making the plain-HTTP page
// impossible to serve in the first place.
func sameOrigin(r *rest.Request, origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Host != "" && parsed.Host == r.Underlying().Host
}
