package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/rest/pkg/rest"
)

// The headers CORS is conducted in.
const (
	originHeader           = "Origin"
	varyHeader             = "Vary"
	allowOriginHeader      = "Access-Control-Allow-Origin"
	allowCredentialsHeader = "Access-Control-Allow-Credentials"
	allowMethodsHeader     = "Access-Control-Allow-Methods"
	allowHeadersHeader     = "Access-Control-Allow-Headers"
	exposeHeadersHeader    = "Access-Control-Expose-Headers"
	maxAgeHeader           = "Access-Control-Max-Age"
	requestMethodHeader    = "Access-Control-Request-Method"
)

// credentialsAllowed is the only value Access-Control-Allow-Credentials takes.
const credentialsAllowed = "true"

// CORS tells a browser whether a page from another origin may read this
// application's responses.
//
// # What it does not do
//
// ⚠ It refuses nothing. CORS is enforced by the browser, not by the server: the
// headers below are an instruction to the browser, and a caller that is not a
// browser ignores them entirely — including a script that simply omits the
// Origin header. A server-side refusal would therefore stop honest cross-origin
// frontends while stopping nothing that meant harm, and it would refuse safe
// requests, which is not what the browser's own rule does.
//
// What does refuse is the cross-origin check (CSRF, in this same package), and
// it refuses the requests that are worth refusing: the ones that change state.
// The two read the same Origins, so they cannot disagree about whose frontend
// this is — but only one of them says no, with one error shape.
//
// # The two kinds of request
//
// A preflight (OPTIONS carrying Access-Control-Request-Method) is answered here
// and never reaches a handler: it asks what would be permitted, and nothing in
// the application has an opinion about that. Every other request is served
// normally and gets the headers attached to whatever it answered.
//
// ⚠ Both halves are needed. Headers on the preflight alone let the browser send
// the request and then block the response, which looks like a network failure
// in the console and like a perfectly served 200 in the server's log.
func CORS(cfg *appconfig.CORS) rest.Middleware {
	origins := ParseOrigins(cfg.AllowOrigin())

	// A same-origin deployment names nothing, and nothing cross-origin is meant
	// to work. Returning the handler untouched keeps it out of the chain rather
	// than running a middleware that would add no header to any response.
	if !origins.Configured() {
		return func(next rest.Handler) rest.Handler { return next }
	}

	return func(next rest.Handler) rest.Handler {
		return func(r *rest.Request) *rest.Response {
			origin, _ := r.Header(originHeader)
			allowed := origins.AllowedValue(origin)

			if isPreflight(r) {
				return preflightResponse(cfg, origins, allowed)
			}

			res := next(r)
			applyHeaders(res, actualHeaders(cfg, origins, allowed))
			return res
		}
	}
}

// isPreflight reports whether the request is the browser asking permission
// rather than doing something.
//
// The method alone is not the test: an application is allowed to serve OPTIONS
// itself, and a preflight is the one that names the method it is asking about.
func isPreflight(r *rest.Request) bool {
	if r.Method() != http.MethodOptions {
		return false
	}
	method, ok := r.Header(requestMethodHeader)
	return ok && method != ""
}

// preflightResponse answers the browser's question.
//
// An origin this deployment does not claim gets an answer with no CORS headers
// at all, which the browser reads as "no" — rather than a 403, which would say
// the same thing to the browser and something false to everyone else.
func preflightResponse(cfg *appconfig.CORS, origins *Origins, allowed string) *rest.Response {
	headers := actualHeaders(cfg, origins, allowed)

	// The rest of the answer is only meaningful when there is a yes to qualify.
	if allowed != "" {
		if methods := cfg.AllowMethods(); methods != "" {
			headers[allowMethodsHeader] = methods
		}
		if allowHeaders := cfg.AllowHeaders(); allowHeaders != "" {
			headers[allowHeadersHeader] = allowHeaders
		}
		if maxAge := cfg.MaxAge(); maxAge > 0 {
			headers[maxAgeHeader] = strconv.Itoa(maxAge)
		}
	}

	return &rest.Response{Code: http.StatusOK, AddHeaders: headers}
}

// actualHeaders are the headers that belong on a real response: whether the
// browser may read it, and what it may read of it.
//
// Access-Control-Allow-Methods and -Headers are deliberately absent. They answer
// a preflight's question about what would be permitted, and a browser ignores
// them anywhere else.
func actualHeaders(cfg *appconfig.CORS, origins *Origins, allowed string) map[string]string {
	headers := map[string]string{}

	// Vary is written whether or not this particular origin was allowed, and
	// that is the point: a cache that stored the header-less answer given to an
	// unknown origin would otherwise replay it to the frontend.
	if origins.VariesByOrigin() {
		headers[varyHeader] = originHeader
	}
	if allowed == "" {
		return headers
	}

	headers[allowOriginHeader] = allowed
	if exposed := cfg.ExposeHeaders(); exposed != "" {
		headers[exposeHeadersHeader] = exposed
	}

	// ⚠ Credentials are never offered alongside the wildcard. A browser refuses
	// that combination outright, so a deployment that wants cookies from
	// another origin has to name it — which is the same conclusion the startup
	// checks reach for cookie authentication.
	if cfg.AllowCredentials() && allowed != wildcardOrigin {
		headers[allowCredentialsHeader] = credentialsAllowed
	}
	return headers
}

// applyHeaders puts the CORS headers on a response the application produced.
//
// ⚠ A handler that set ReplaceHeaders has asked for the base headers to be
// cleared, and the pipeline then ignores AddHeaders entirely — so the headers
// have to go into whichever map that response is actually going to use, or they
// are silently dropped for exactly the endpoints that customise their headers.
func applyHeaders(res *rest.Response, headers map[string]string) {
	if res == nil || len(headers) == 0 {
		return
	}

	target := res.AddHeaders
	if res.ReplaceHeaders != nil {
		target = res.ReplaceHeaders
	}
	if target == nil {
		target = make(map[string]string, len(headers))
		res.AddHeaders = target
	}
	for name, value := range headers {
		existing, taken := target[name]
		switch {
		case !taken:
			target[name] = value
		case name == varyHeader:
			// ⚠ Vary is a list, not a value, so the handler's own must not
			// simply win. A response that said `Vary: Accept-Encoding` and lost
			// `Origin` is cacheable across origins — and the cache would then
			// serve one origin's Access-Control-Allow-Origin to another, which
			// is the exact failure Vary is written to prevent.
			target[name] = appendVary(existing, value)
		default:
			// Every other header the handler set, it meant. An endpoint that
			// wrote its own Access-Control-Allow-Origin had a reason.
		}
	}
}

// appendVary adds a field name to a Vary header, leaving one that is already
// listed alone. The comparison is case-insensitive because header names are.
func appendVary(existing, name string) string {
	for _, field := range strings.Split(existing, originSeparator) {
		if strings.EqualFold(strings.TrimSpace(field), name) {
			return existing
		}
	}
	return existing + ", " + name
}
