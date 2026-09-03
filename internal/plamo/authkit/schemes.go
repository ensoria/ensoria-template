package authkit

import "github.com/ensoria/config/pkg/appconfig"

// ConfiguredSchemes reports which kinds of credential the configuration
// describes, in a stable order.
//
// This is what the parts that can only read configuration go by — chiefly
// describe, which builds the API documentation without starting the
// application. API keys count when they are listed in the configuration and
// when it declares they are verified elsewhere (AUTH_API_KEYS_EXTERNAL),
// because a document that omitted the key scheme would be wrong either way.
//
// A running application asks its Verifier instead: only the verifier knows
// about a key store handed to it by application code.
func ConfiguredSchemes(cfg *appconfig.Auth) []string {
	if cfg == nil {
		return nil
	}
	var schemes []string
	if cfg.Secret != "" || cfg.JWKSURL != "" {
		schemes = append(schemes, SchemeJWT)
	}
	if cfg.AcceptsAPIKeys() {
		schemes = append(schemes, SchemeAPIKey)
	}
	if cfg.UsesSessions() {
		schemes = append(schemes, SchemeSession)
	}
	return schemes
}
