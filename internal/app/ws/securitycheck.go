package ws

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
)

// checkChannelSecurity reports a set of channels whose declarations cannot be
// served as written. It is the WebSocket counterpart of the HTTP startup check,
// and refuses the same three things:
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
	var contradictory, needCaller []string
	for _, m := range modules {
		security := m.Security()
		if security.Contradictory() {
			contradictory = append(contradictory, m.RuntimeModule().Path)
		}
		if security.RequiresCaller() {
			needCaller = append(needCaller, m.RuntimeModule().Path)
		}
	}
	if len(contradictory) > 0 {
		return fmt.Errorf("WebSocket channels %s are declared public and also require a scheme or a scope, "+
			"which a public channel never checks: drop Public to require them, or drop Schemes and Scopes to admit everyone",
			quoteList(contradictory))
	}
	if len(needCaller) == 0 {
		return nil
	}

	var verifiable []string
	if verifier != nil {
		verifiable = verifier.Schemes()
	}
	if len(verifiable) == 0 {
		return fmt.Errorf("WebSocket channels %s require an authenticated caller but nothing can verify one: "+
			"set AUTH_MODE and AUTH_SECRET (or AUTH_JWKS_URL), AUTH_API_KEYS (or a key store of your own), "+
			"or AUTH_SESSION_STORE, or declare the channels public with Channel.Security",
			quoteList(needCaller))
	}

	var unverifiable []string
	for _, m := range modules {
		security := m.Security()
		if security == nil || security.Public {
			continue
		}
		for _, scheme := range security.Schemes {
			if !slices.Contains(verifiable, scheme) {
				unverifiable = append(unverifiable, fmt.Sprintf("%s (%q)", m.RuntimeModule().Path, scheme))
			}
		}
	}
	if len(unverifiable) > 0 {
		return fmt.Errorf("WebSocket channels accept only a credential nothing verifies: %s (available: %s): "+
			"configure it, or widen Channel.Security.Schemes",
			strings.Join(unverifiable, ", "), quoteList(verifiable))
	}
	return nil
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
