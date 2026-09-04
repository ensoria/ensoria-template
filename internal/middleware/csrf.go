package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// LogTypeCrossOriginDenied labels the record written when a request is refused
// for where it came from.
//
// It is a constant for the reason the authkit log types are: a log platform is
// given a search and an alert condition that has to survive a change of
// wording, and the test and that condition should read the same value.
//
// ⚠ A handful of these is ordinary — a frontend deployed on a new origin nobody
// added to the configuration produces them until someone does. A sudden stream
// of them from origins nobody recognises is the thing worth an alert.
const LogTypeCrossOriginDenied = "cross_origin_denied_log"

// CrossOriginChecker judges whether a request may be served, given where the
// browser says it came from.
//
// It is an interface so the check can be replaced. net/http's implementation is
// what this template installs and what almost every project should keep, but a
// project with a genuinely unusual topology — an embedded webview, a gateway
// that rewrites Origin — needs somewhere to put its own rule, and a seam it can
// use is better than a fork of the middleware.
type CrossOriginChecker interface {
	// Check reports whether the request must be refused. A nil error serves it.
	Check(r *http.Request) error
}

// NewCrossOriginProtection builds the cross-origin check from the origins the
// CORS configuration already names.
//
// ⚠ The trusted origins are deliberately not a setting of their own. Both
// answer one question — which other origin is this application's own frontend —
// and two keys holding one fact disagree eventually, in the direction where
// CORS lets a page make the call and this refuses it, or the reverse.
//
// The origins come from ParseOrigins, the one reading of CORS_ALLOW_ORIGIN that
// CORS uses as well. An empty set is the same-origin deployment, and it needs no
// trusted origins — a request from the origin serving the page is not
// cross-origin in the first place.
func NewCrossOriginProtection(origins *Origins) (*http.CrossOriginProtection, error) {
	protection := http.NewCrossOriginProtection()

	// Named rather than every configured value: the wildcard is dropped, not
	// expanded. The whole question this middleware answers is which sites may
	// make a browser send a state-changing request with the user's cookie
	// attached, and "all of them" is not an answer to it. A deployment that
	// combines the wildcard with cookie authentication is refused at startup
	// (checkTrustedOrigins in internal/app/http), so reaching here with one set
	// means sessions are off.
	for _, origin := range origins.Named() {
		if err := protection.AddTrustedOrigin(origin); err != nil {
			return nil, fmt.Errorf("CORS_ALLOW_ORIGIN names %q, which is not an origin a browser can send "+
				"(it has to be scheme://host with an optional port, and no trailing slash or path): %w",
				origin, err)
		}
	}
	return protection, nil
}

// CSRF refuses a state-changing request that a browser made from somewhere this
// application does not trust.
//
// # Why this is here at all
//
// A cookie is attached by the browser to every request to this origin, whoever
// caused it. So a form on another site can make the browser send an
// authenticated POST — the credential rides along without anyone choosing to
// send it. That is the one weakness cookies have and bearer tokens do not, and
// it is the price of a credential JavaScript cannot read.
//
// # Why every request, not only the authenticated ones
//
// The check does not ask whether the request carried a session. Narrowing it to
// cookie-authenticated requests would mean the protection depends on a
// condition evaluated per request, and the day somebody adds a second thing the
// browser sends by itself, this is a hole. Callers that are not browsers —
// a server, a CLI, a mobile client — send neither Sec-Fetch-Site nor Origin and
// are allowed through untouched, so applying it everywhere costs them nothing.
//
// # What it does not do
//
// GET, HEAD and OPTIONS are always allowed: they are the safe methods, and the
// protection is built on the assumption that they change nothing. An endpoint
// that changes state on GET is outside what this can defend, which is one more
// reason not to write one.
//
// ⚠ A WebSocket upgrade is a GET, so it is not covered here. Its origin is
// checked at the upgrade instead (see the WebSocket router's own check).
func CSRF(checker CrossOriginChecker) rest.Middleware {
	return func(next rest.Handler) rest.Handler {
		return func(r *rest.Request) *rest.Response {
			if checker == nil {
				return next(r)
			}
			if err := checker.Check(r.Underlying()); err != nil {
				logCrossOriginDenial(r, err)
				return restkit.CrossOriginResponse()
			}
			return next(r)
		}
	}
}

// logCrossOriginDenial records a refused request.
//
// The origin the browser claimed is the field worth having: it is what decides
// whether this is a frontend somebody forgot to configure or a site attacking
// the application, and it is the value that goes into CORS_ALLOW_ORIGIN if the
// answer is the first one.
func logCrossOriginDenial(r *rest.Request, err error) {
	origin, _ := r.Header("Origin")
	loggear.Warn("a request was refused for the origin it came from",
		slog.String("type", LogTypeCrossOriginDenied),
		slog.String("origin", origin),
		slog.String("method", r.Method()),
		slog.String("path", r.Path()),
		slog.String("remote_addr", r.RemoteAddr()),
		slog.String("error", err.Error()),
	)
}
