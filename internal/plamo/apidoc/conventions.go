package apidoc

import "reflect"

// Conventions は API 全体の共通規約(docai CONVENTIONS.md の素材)。
// 実行時 config / pipeline 由来の値(BaseURLs/CORS/GlobalMiddlewares/AuthMethod)は
// describe 実行時(Phase 7)に populate する。CommonError は型から組める。
type Conventions struct {
	BaseURLs map[string]string `json:"base_urls,omitempty"` // 環境名 → ベース URL
	// SecuritySchemes は呼び出し元が資格情報を提示できる方式。設定から組み立てる。
	SecuritySchemes []SecurityScheme `json:"security_schemes,omitempty"`
	// AuthMethod は上記で表せない補足を書く自由記述欄(任意)。
	AuthMethod        string   `json:"auth_method,omitempty"`
	CORS              *CORS    `json:"cors,omitempty"`
	CommonError       *Schema  `json:"common_error,omitempty"` // 全エンドポイント共通のエラー本文形
	GlobalMiddlewares []string `json:"global_middlewares,omitempty"`
}

// SecurityScheme は資格情報の提示方式1つ。
// Name は Endpoint.Security.Schemes が参照する識別子(authkit.SchemeJWT など)で、
// OpenAPI の components.securitySchemes のキーにもそのまま使う。
type SecurityScheme struct {
	Name string `json:"name"`
	// Type は "http"(Authorization ヘッダ)か "apiKey"(任意のヘッダ)。
	Type string `json:"type"`
	// Scheme は Type=="http" のときの認証スキーム(例 "bearer")。
	Scheme string `json:"scheme,omitempty"`
	// BearerFormat は bearer トークンの形式(例 "JWT")。
	BearerFormat string `json:"bearer_format,omitempty"`
	// In と HeaderName は Type=="apiKey" のときの受け取り場所。
	// In が "cookie" のとき、HeaderName は Cookie 名を指す。
	In         string `json:"in,omitempty"`
	HeaderName string `json:"header_name,omitempty"`
	// Description は呼び出し元向けの補足。
	Description string `json:"description,omitempty"`
}

// SecurityScheme の Type / In / Scheme に使う値(OpenAPI の語彙に合わせる)。
const (
	SecuritySchemeTypeHTTP   = "http"
	SecuritySchemeTypeAPIKey = "apiKey"
	SecuritySchemeInHeader   = "header"
	SecuritySchemeInCookie   = "cookie"
	SecuritySchemeBearer     = "bearer"
	BearerFormatJWT          = "JWT"
)

// CORS は CONVENTIONS の CORS 規約(config の文字列表現をそのまま持つ)。
type CORS struct {
	AllowOrigin      string `json:"allow_origin,omitempty"`
	AllowMethods     string `json:"allow_methods,omitempty"`
	AllowHeaders     string `json:"allow_headers,omitempty"`
	ExposeHeaders    string `json:"expose_headers,omitempty"`
	AllowCredentials bool   `json:"allow_credentials,omitempty"`
	MaxAge           int    `json:"max_age,omitempty"`
}

// CommonErrorSchema は共通エラー本文の型からスキーマ + example を組む。
// describe 側で共通エラー型(例: dto.Error)を渡して Conventions.CommonError に入れる。
func CommonErrorSchema(t reflect.Type) *Schema {
	s := SchemaFromType(t)
	if s != nil {
		s.Example = ExampleFromType(t, nil, ExampleOptions{})
	}
	return s
}
