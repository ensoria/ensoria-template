package http

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// checkAuthConfiguration reports a configuration that cannot serve the
// endpoints it is given.
//
// Both failures below would otherwise show up as every request being refused,
// with nothing to say why. They are settled at startup instead, while the
// person who can fix them is watching.
//
// It returns an error rather than stopping the process itself so the rule can
// be tested.
func checkAuthConfiguration(modules []*rest.Module, auth *appconfig.Auth) error {
	if !restkit.RequiresAuthentication(modules) {
		return nil
	}

	if !auth.Configured() {
		return fmt.Errorf("endpoints require an authenticated caller but no authentication is " +
			"configured: set AUTH_MODE and AUTH_SECRET (or AUTH_JWKS_URL), or AUTH_API_KEYS, " +
			"or declare the endpoints public with Endpoint.Security")
	}

	// An endpoint may insist on one kind of credential. Requiring a kind the
	// configuration cannot verify closes the endpoint to everyone.
	configured := authkit.ConfiguredSchemes(auth)
	var missing []string
	for _, scheme := range restkit.DeclaredSchemes(modules) {
		if !slices.Contains(configured, scheme) {
			missing = append(missing, scheme)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("endpoints accept only the %s credential, which nothing verifies "+
			"(configured: %s): configure it, or widen Endpoint.Security.Schemes",
			quoteList(missing), quoteList(configured))
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
