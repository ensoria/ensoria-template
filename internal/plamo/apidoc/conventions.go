package apidoc

import "github.com/ensoria/encli/pkg/apicontract"

import "reflect"

// Conventions は API 全体の共通規約(docai CONVENTIONS.md の素材)。
// 実行時 config / pipeline 由来の値(BaseURLs/CORS/GlobalMiddlewares/AuthMethod)は
// describe 実行時(Phase 7)に populate する。CommonError は型から組める。
type Conventions struct {
	BaseURLs map[string]string `json:"base_urls,omitempty"` // 環境名 → ベース URL
	// SecuritySchemes は呼び出し元が資格情報を提示できる方式。設定から組み立てる。
	SecuritySchemes []SecurityScheme `json:"security_schemes,omitempty"`
	// ScopeImplications maps a scope to the scopes it stands for.
	//
	// Without it a generated document is misleading rather than merely
	// incomplete: an endpoint declaring `orders:write` says nothing about admin,
	// so a reader holding admin would conclude they cannot call it. The
	// declaration is right to stay silent — which scope implies which is the
	// application's policy, not the endpoint's — so the policy is published
	// once, here.
	ScopeImplications map[string][]string `json:"scope_implications,omitempty"`
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
	// In は Type=="apiKey" の資格情報が乗る場所("header" / "cookie" / "query")。
	In string `json:"in,omitempty"`
	// ParameterName はその乗り物の名前 —— In に応じてヘッダ名・Cookie 名・
	// クエリパラメータ名を指す。
	//
	// Cookie のスキームができるまでは HeaderName という名前だったが、それでは
	// 値の半分について嘘になる(セッション ID を「ヘッダに入れろ」と読める文書が
	// 生成される)。出力先の2形式はどちらもこの値を `name` と呼び、何の名前かは
	// `in` に決めさせているので、中立な名前がそのまま忠実な写しにもなる。
	ParameterName string `json:"parameter_name,omitempty"`
	// Description は呼び出し元向けの補足。
	Description string `json:"description,omitempty"`
}

// SecurityScheme の Type / In / Scheme に使う値(OpenAPI の語彙に合わせる)。
//
// 値そのものは [apicontract] が持つ。ここはその再輸出であり、
// **読み取り側(encli のレンダラ)と同じ定数を指している**。
// この JSON を書く側と読む側は別モジュールで互いのソースを見られないので、
// レンダラが分岐に使う文字列を両側で綴ると、食い違ったときに誰も気づけない
// —— フィールドは届いているので、既定値に落ちた文書が黙って出るだけになる。
// 詳しい理由は apicontract のパッケージコメントを参照。
const (
	SecuritySchemeTypeHTTP   = apicontract.SchemeTypeHTTP
	SecuritySchemeTypeAPIKey = apicontract.SchemeTypeAPIKey
	SecuritySchemeInHeader   = apicontract.InHeader
	SecuritySchemeInCookie   = apicontract.InCookie
	SecuritySchemeInQuery    = apicontract.InQuery
	SecuritySchemeBearer     = apicontract.SchemeBearer
	BearerFormatJWT          = apicontract.BearerFormatJWT
)

// GlobalMiddlewares に載せる名前。
//
// この一覧のうち **MiddlewareCSRF だけが生成側との契約**である。Cookie で
// 資格情報を運ぶ API の生成ドキュメントは、このミドルウェアが実際に名前を
// 連ねているときだけ「サーバがクロスサイトの状態変更を拒否する」と書く。
// 外したアプリが、持っていない保護を約束されないようにするための照合であり、
// 綴りが食い違うと**保護は生きたままドキュメントからだけ消える**
// —— しかも、読み手には安心して見える方向に。
//
// 残りはこのアプリケーションが自分で決める名前で、生成側は素通しするだけ。
// プロジェクトが独自のミドルウェアを足すなら、ここに好きな名前を足してよい
// —— 共有する必要は無い。逆に言えば、**生成側が新しく分岐に使う名前を覚えたら、
// その名前はそのときに apicontract へ移す**。
const (
	MiddlewareCSRF = apicontract.MiddlewareCSRF

	MiddlewareLogging            = "logging"
	MiddlewareRecovery           = "recovery"
	MiddlewareVerifyBodyParsable = "verify-body-parsable"
	MiddlewareCORS               = "cors"
	MiddlewareAuth               = "auth"
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
