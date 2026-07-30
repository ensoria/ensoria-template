package authkit

import "github.com/ensoria/config/pkg/appconfig"

// ConfiguredSchemes reports which kinds of credential the configuration can
// actually verify, in a stable order.
//
// This is the one place that answers "can this application check a JWT / an API
// key?". The startup checks, the generated documentation and the verifier all
// read the same answer, so an endpoint can never require a credential the
// documentation claims is available but nothing verifies.
func ConfiguredSchemes(cfg *appconfig.Auth) []string {
	if cfg == nil {
		return nil
	}
	var schemes []string
	if cfg.Secret != "" || cfg.JWKSURL != "" {
		schemes = append(schemes, SchemeJWT)
	}
	if len(cfg.APIKeys) > 0 {
		schemes = append(schemes, SchemeAPIKey)
	}
	return schemes
}
