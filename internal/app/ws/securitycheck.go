package ws

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
)

// checkChannelSecurity reports the channels whose declarations cannot be served
// as written. It is the WebSocket counterpart of the HTTP startup check, and
// refuses the same three things, in this order:
//
//   - a public channel that also states a scheme or a scope, which it never
//     checks: it would admit everyone while declaring that it does not;
//   - a channel that needs a caller in an application that can verify none,
//     which would refuse every handshake with nothing to say why;
//   - a channel that accepts only kinds of credential nothing verifies, which
//     closes it to everyone.
//
// The question of what can be verified is put to the verifier rather than to
// the configuration, for the reason the HTTP check gives: a key store supplied
// by application code never appears in the configuration.
func checkChannelSecurity(modules []*wskit.Module, verifier authkit.Verifier) error {
	// First, because it is about the declarations alone. The checks below ask
	// whether the configuration can serve what is declared, and an application
	// whose channels are all public would skip them at the early return.
	if paths := contradictoryChannels(modules); len(paths) > 0 {
		return fmt.Errorf("WebSocket channels %s are declared public and also require a scheme or a scope, "+
			"which a public channel never checks: drop Public to require them, or drop Schemes and Scopes to admit everyone",
			quoteList(paths))
	}

	paths := channelsRequiringCaller(modules)
	if len(paths) == 0 {
		return nil
	}

	verifiable := verifiableSchemes(verifier)
	if len(verifiable) == 0 {
		return fmt.Errorf("WebSocket channels %s require an authenticated caller but nothing can verify one: "+
			"set AUTH_MODE and AUTH_SECRET (or AUTH_JWKS_URL), AUTH_API_KEYS (or a key store of your own), "+
			"or AUTH_SESSION_STORE, or declare the channels public with Channel.Security",
			quoteList(paths))
	}

	if unverifiable := unverifiableSchemes(modules, verifiable); len(unverifiable) > 0 {
		return fmt.Errorf("WebSocket channels accept only a credential nothing verifies: %s (available: %s): "+
			"configure it, or widen Channel.Security.Schemes",
			strings.Join(unverifiable, ", "), quoteList(verifiable))
	}
	return nil
}

// contradictoryChannels returns the paths of the public channels that also
// state a condition, in the order the modules were given.
func contradictoryChannels(modules []*wskit.Module) []string {
	var paths []string
	for _, m := range modules {
		if m.Security().Contradictory() {
			paths = append(paths, m.RuntimeModule().Path)
		}
	}
	return paths
}

// channelsRequiringCaller returns the paths of the channels that admit only a
// verified caller — every one not declared Public, including those that
// declare nothing — in the order the modules were given.
func channelsRequiringCaller(modules []*wskit.Module) []string {
	var paths []string
	for _, m := range modules {
		if m.Security().RequiresCaller() {
			paths = append(paths, m.RuntimeModule().Path)
		}
	}
	return paths
}

// verifiableSchemes returns the kinds of credential the application can
// verify. Having no verifier at all means it can verify none.
func verifiableSchemes(verifier authkit.Verifier) []string {
	if verifier == nil {
		return nil
	}
	return verifier.Schemes()
}

// unverifiableSchemes names each kind of credential a channel accepts only
// that is missing from verifiable, as `"/ws/path" ("apiKey")`, in the order
// the modules declare them.
func unverifiableSchemes(modules []*wskit.Module, verifiable []string) []string {
	var unverifiable []string
	for _, m := range modules {
		for _, scheme := range limitedSchemes(m.Security()) {
			if !slices.Contains(verifiable, scheme) {
				unverifiable = append(unverifiable, fmt.Sprintf("%q (%q)", m.RuntimeModule().Path, scheme))
			}
		}
	}
	return unverifiable
}

// limitedSchemes returns the kinds of credential a declaration limits its
// channel to. It is nil both for a channel that declares nothing, which
// accepts any kind the application verifies, and for a public one, which
// accepts no credential in particular.
func limitedSchemes(security *wskit.SecuritySpec) []string {
	if security == nil || security.Public {
		return nil
	}
	return security.Schemes
}

// quoteList renders names for an error message. An empty list reads as "none"
// rather than as an empty gap in the sentence.
func quoteList(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, `"`+n+`"`)
	}
	return strings.Join(quoted, ", ")
}
