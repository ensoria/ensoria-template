package describe

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// securitySchemes は設定されている検証手段から、呼び出し元が使える資格情報の方式を組む。
// 設定されていない方式は出さない —— 使えない認証方法をドキュメントに載せないため。
//
// API キーは、設定に並んでいる場合と「別の場所で検証する」と宣言されている場合の
// 両方で出す。DB から検証するアプリでも、呼び出し元にとっては API キーが使えることに
// 変わりないため。
//
// Both surfaces describe the same schemes, so this sits apart from http.go and
// messaging.go rather than inside either of them.
func securitySchemes(auth *appconfig.Auth) []apidoc.SecurityScheme {
	if auth == nil {
		return nil
	}

	var schemes []apidoc.SecurityScheme
	if auth.Secret != "" || auth.JWKSURL != "" {
		schemes = append(schemes, apidoc.SecurityScheme{
			Name:         authkit.SchemeJWT,
			Type:         apidoc.SecuritySchemeTypeHTTP,
			Scheme:       apidoc.SecuritySchemeBearer,
			BearerFormat: apidoc.BearerFormatJWT,
			Description:  "Bearer token issued by the identity provider",
		})
	}
	if auth.AcceptsAPIKeys() {
		header := auth.APIKeyHeader
		if header == "" {
			header = appconfig.DefaultAPIKeyHeader
		}
		schemes = append(schemes, apidoc.SecurityScheme{
			Name:        authkit.SchemeAPIKey,
			Type:        apidoc.SecuritySchemeTypeAPIKey,
			In:          apidoc.SecuritySchemeInHeader,
			HeaderName:  header,
			Description: "Key issued to a machine caller",
		})
	}
	return schemes
}
