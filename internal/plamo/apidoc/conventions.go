package apidoc

import "reflect"

// Conventions は API 全体の共通規約(docai CONVENTIONS.md の素材)。
// 実行時 config / pipeline 由来の値(BaseURLs/CORS/GlobalMiddlewares/AuthMethod)は
// describe 実行時(Phase 7)に populate する。CommonError は型から組める。
type Conventions struct {
	BaseURLs          map[string]string `json:"base_urls,omitempty"` // 環境名 → ベース URL
	AuthMethod        string            `json:"auth_method,omitempty"`
	CORS              *CORS             `json:"cors,omitempty"`
	CommonError       *Schema           `json:"common_error,omitempty"` // 全エンドポイント共通のエラー本文形
	GlobalMiddlewares []string          `json:"global_middlewares,omitempty"`
}

// CORS は CONVENTIONS の CORS 規約。
type CORS struct {
	AllowOrigin      []string `json:"allow_origin,omitempty"`
	AllowMethods     []string `json:"allow_methods,omitempty"`
	AllowHeaders     []string `json:"allow_headers,omitempty"`
	ExposeHeaders    []string `json:"expose_headers,omitempty"`
	AllowCredentials bool     `json:"allow_credentials,omitempty"`
	MaxAge           int      `json:"max_age,omitempty"`
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
