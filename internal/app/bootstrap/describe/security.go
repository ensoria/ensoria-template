package describe

import (
	"fmt"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// securitySchemes は設定されている検証手段から、呼び出し元が使える資格情報の方式を組む。
// 設定されていない方式は出さない —— 使えない認証方法をドキュメントに載せないため。
//
// Which schemes a configuration enables is not decided here. That question is
// answered once, by authkit.ConfiguredSchemes, and this walks its answer. The
// conditions used to be written out a second time in this file, which meant a
// new credential kind had to be remembered in two places that nothing compared:
// the generated document could then describe an application that accepts
// something else, or stay silent about something it does accept.
//
// What is left here is the part that genuinely belongs to documentation: how a
// caller presents each credential (which header, which cookie, what the value
// looks like).
//
// Both surfaces describe the same schemes, so this sits apart from http.go and
// messaging.go rather than inside either of them.
func securitySchemes(auth *appconfig.Auth) []apidoc.SecurityScheme {
	names := authkit.ConfiguredSchemes(auth)
	if len(names) == 0 {
		return nil
	}

	schemes := make([]apidoc.SecurityScheme, 0, len(names))
	for _, name := range names {
		schemes = append(schemes, describeScheme(name, auth))
	}
	return schemes
}

// describeScheme renders one credential kind as the thing a caller reads: where
// to put the credential, and what it is.
//
// An unrecognized name panics rather than being skipped. Reaching that branch
// means authkit has learned about a credential this file has not, and the
// alternatives are both worse than stopping: dropping it publishes a document
// that stays silent about a way in, and inventing a descriptor publishes one
// that tells callers to do the wrong thing. This runs in the describe program,
// so the failure lands on whoever is generating documentation — which is
// whoever can fix it.
func describeScheme(name string, auth *appconfig.Auth) apidoc.SecurityScheme {
	switch name {
	case authkit.SchemeJWT:
		return apidoc.SecurityScheme{
			Name:         authkit.SchemeJWT,
			Type:         apidoc.SecuritySchemeTypeHTTP,
			Scheme:       apidoc.SecuritySchemeBearer,
			BearerFormat: apidoc.BearerFormatJWT,
			Description:  "Bearer token issued by the identity provider",
		}
	case authkit.SchemeAPIKey:
		return apidoc.SecurityScheme{
			Name:        authkit.SchemeAPIKey,
			Type:        apidoc.SecuritySchemeTypeAPIKey,
			In:          apidoc.SecuritySchemeInHeader,
			HeaderName:  apiKeyHeader(auth),
			Description: "Key issued to a machine caller",
		}
	default:
		panic(fmt.Sprintf(
			"describe: no descriptor for the %q credential scheme. "+
				"authkit.ConfiguredSchemes reports it, so add its descriptor to describeScheme in %s",
			name, securityFile))
	}
}

// apiKeyHeader is the header an API key is read from, falling back to the
// framework default the verifier falls back to.
func apiKeyHeader(auth *appconfig.Auth) string {
	if auth != nil && auth.APIKeyHeader != "" {
		return auth.APIKeyHeader
	}
	return appconfig.DefaultAPIKeyHeader
}

// securityFile is where the panic above sends the reader. It is this file; the
// path is written out because whoever sees the message is looking at a
// generator's output rather than at this package.
const securityFile = "internal/app/bootstrap/describe/security.go"
