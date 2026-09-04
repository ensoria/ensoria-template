package middleware

import "strings"

// wildcardOrigin is the CORS_ALLOW_ORIGIN value that means "any site".
const wildcardOrigin = "*"

// originSeparator is what CORS_ALLOW_ORIGIN separates several origins with.
const originSeparator = ","

// Origins is the set of origins a deployment calls its own frontend, read from
// CORS_ALLOW_ORIGIN.
//
// It exists because three separate things ask that one question and must not
// answer it differently:
//
//   - CORS, which tells the browser whether it may read the response;
//   - the cross-origin check, which refuses a state-changing request the
//     browser says came from somewhere else;
//   - the WebSocket upgrade, which cannot use the cross-origin check because an
//     upgrade is a GET and GET is always allowed there.
//
// Parsing the setting three times would let them drift in the direction where
// one lets a page through and another refuses it — a class of bug that shows up
// only in a browser, and only for some origins.
type Origins struct {
	// wildcard is CORS_ALLOW_ORIGIN=*: every site is claimed as the frontend.
	//
	// ⚠ It is refused at startup together with cookie authentication (see
	// checkTrustedOrigins in internal/app/http). Reaching here with it set means
	// the application authenticates with tokens only, where a wildcard is an
	// ordinary open API.
	wildcard bool
	// list is the origins written out, in the order they were written.
	list []string
}

// ParseOrigins reads CORS_ALLOW_ORIGIN as a deployment writes it: a
// comma-separated list, the wildcard, or nothing at all.
//
// Nothing at all is the same-origin deployment — the frontend is served by this
// application — and it claims no other origin. That is not the same as the
// wildcard, which claims all of them.
func ParseOrigins(allowOrigin string) *Origins {
	origins := &Origins{}
	for _, origin := range strings.Split(allowOrigin, originSeparator) {
		origin = strings.TrimSpace(origin)
		switch origin {
		case "":
			continue
		case wildcardOrigin:
			origins.wildcard = true
		default:
			origins.list = append(origins.list, origin)
		}
	}
	return origins
}

// Wildcard reports whether every origin is claimed.
func (o *Origins) Wildcard() bool { return o != nil && o.wildcard }

// Configured reports whether the deployment named anything at all. False is the
// same-origin deployment, where nothing cross-origin is meant to work.
func (o *Origins) Configured() bool {
	return o != nil && (o.wildcard || len(o.list) > 0)
}

// Named returns the origins written out, the wildcard excluded.
//
// It is what the cross-origin check is given: "every site" is not an answer to
// the question that check asks, so the wildcard is dropped rather than expanded.
func (o *Origins) Named() []string {
	if o == nil {
		return nil
	}
	return append([]string(nil), o.list...)
}

// Allows reports whether a request from origin comes from a frontend this
// deployment claims. An empty origin is not a browser request and is never
// claimed by this.
func (o *Origins) Allows(origin string) bool {
	return o.AllowedValue(origin) != ""
}

// AllowedValue is what Access-Control-Allow-Origin should say for a request
// from origin, or "" when the browser is to be told nothing.
//
// ⚠ It answers with **one** origin, never the configured list. The header takes
// a single origin or the wildcard and nothing else, so echoing "a.example,
// b.example" produces a header every browser rejects — while looking, in a
// server-side test, exactly like a working configuration.
func (o *Origins) AllowedValue(origin string) string {
	if o == nil || origin == "" {
		return ""
	}
	if o.wildcard {
		return wildcardOrigin
	}
	for _, allowed := range o.list {
		if allowed == origin {
			return origin
		}
	}
	return ""
}

// VariesByOrigin reports whether the answer depends on which origin asked,
// which is what a cache has to be told with Vary.
//
// The wildcard does not vary: every origin gets the same header.
func (o *Origins) VariesByOrigin() bool {
	return o != nil && !o.wildcard && len(o.list) > 0
}
