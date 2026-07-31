package http

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// checkAuthConfiguration reports a setup that cannot serve the endpoints it is
// given.
//
// Both failures below would otherwise show up as every request being refused,
// with nothing to say why. They are settled at startup instead, while the
// person who can fix them is watching.
//
// The question is put to the verifier rather than to the configuration.
// An application that verifies API keys against a database has none in its
// configuration, and reading the configuration would call it unconfigured.
//
// It returns an error rather than stopping the process itself so the rule can
// be tested.
func checkAuthConfiguration(modules []*rest.Module, verifier authkit.Verifier) error {
	if !restkit.RequiresAuthentication(modules) {
		return nil
	}

	var verifiable []string
	if verifier != nil {
		verifiable = verifier.Schemes()
	}
	if len(verifiable) == 0 {
		return fmt.Errorf("endpoints require an authenticated caller but nothing can verify one: " +
			"set AUTH_MODE and AUTH_SECRET (or AUTH_JWKS_URL), or AUTH_API_KEYS " +
			"(or AUTH_API_KEYS_EXTERNAL with a key store of your own), " +
			"or declare the endpoints public with Endpoint.Security")
	}

	// An endpoint may insist on one kind of credential. Requiring a kind
	// nothing verifies closes the endpoint to everyone.
	var missing []string
	for _, scheme := range restkit.DeclaredSchemes(modules) {
		if !slices.Contains(verifiable, scheme) {
			missing = append(missing, scheme)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("endpoints accept only the %s credential, which nothing verifies "+
			"(available: %s): configure it, or widen Endpoint.Security.Schemes",
			quoteList(missing), quoteList(verifiable))
	}
	return nil
}

// quoteList renders scheme names for an error message. An empty list reads as
// "none" rather than as an empty gap in the sentence.
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
