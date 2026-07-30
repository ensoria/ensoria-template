// Package apidoc は、restkit.Endpoint(型・検証ルール・ルーティング宣言)から
// 出力フォーマット中立の API モデル(APISpec)をリフレクションで組み立てる。
//
// APISpec は docai 専用語彙にせず、HTTP API の一般的な意味論で定義する。
// describe がこれを JSON で出力し、encli(レンダラ)が docai/OpenAPI などへ変換する。
package apidoc

// APISpec は1つの API 全体の中立モデル。
type APISpec struct {
	Info        *Info           `json:"info,omitempty"`
	Endpoints   []*EndpointSpec `json:"endpoints"`
	Conventions *Conventions    `json:"conventions,omitempty"`
}

// Info は API 全体のメタ情報(OpenAPI の `info` に対応)。
// 型からは導けないので、アプリ側の宣言(internal/app/apiinfo)を describe が注入する。
type Info struct {
	Title       string   `json:"title,omitempty"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	License     *License `json:"license,omitempty"`
}

// License は API のライセンス(OpenAPI の `info.license`)。
type License struct {
	// Name はライセンス名。OpenAPI では license を出す場合の必須項目。
	Name string `json:"name,omitempty"`
	// Identifier は SPDX ライセンス式(例 "Apache-2.0")。
	// OpenAPI 3.1 では URL と排他なので、レンダラは両方あれば Identifier を優先する。
	Identifier string `json:"identifier,omitempty"`
	// URL はライセンス文書の URL。SPDX 識別子が無い独自ライセンス向け。
	URL string `json:"url,omitempty"`
}

// EndpointSpec は1エンドポイントの中立モデル。
type EndpointSpec struct {
	Method        string       `json:"method"`
	Path          string       `json:"path"`
	Summary       string       `json:"summary,omitempty"`
	Description   string       `json:"description,omitempty"`
	Task          string       `json:"task,omitempty"`      // INDEX の Task 列(§3.2)
	AlsoRead      []string     `json:"also_read,omitempty"` // INDEX の "Also read" 列(§3.2)
	Related       []string     `json:"related,omitempty"`   // §4.1 `### Related`
	PathParams    []PathParam  `json:"path_params,omitempty"`
	QueryParams   []QueryParam `json:"query_params,omitempty"`
	SuccessStatus int          `json:"success_status,omitempty"`
	Request       *Schema      `json:"request,omitempty"`
	Response      *Schema      `json:"response,omitempty"`
	// ResponseMediaType は成功レスポンスの Content-Type を固定する場合の値
	// (Endpoint.Produces。空=既定 application/json)。
	ResponseMediaType string           `json:"response_media_type,omitempty"`
	ResponseHeaders   []ResponseHeader `json:"response_headers,omitempty"`
	// Responses は主レスポンス(SuccessStatus + Response)以外の成功レスポンス。
	// 状況で異なる成功ステータスを返す場合(200/201 併存、202 + 別ボディ型など)に宣言する。
	Responses []ResponseSpec `json:"responses,omitempty"`
	Errors    []ErrorSpec    `json:"errors,omitempty"` // エンドポイント固有エラー(§4.1)
	// Security は誰が呼べるか。nil は「検証済みの呼び出し元が必要」を意味する
	// (宣言漏れは閉じる側に倒れる)。
	Security *Security `json:"security,omitempty"`
	Behavior Behavior  `json:"behavior"`
	// Untyped は Documented を実装しない生 Controller(型不明)のとき true。
	Untyped bool `json:"untyped,omitempty"`
}

// ResponseSpec は主レスポンス以外の成功レスポンス。
// Body が nil のときは本文なし(204 等)。When は「どういうときにこれが返るか」の散文で、
// 型からは導けないため宣言が唯一の出所。
type ResponseSpec struct {
	Status  int              `json:"status"`
	When    string           `json:"when,omitempty"`
	Body    *Schema          `json:"body,omitempty"`
	Headers []ResponseHeader `json:"headers,omitempty"`
}

// ErrorSpec はエンドポイント固有エラーの中立モデル(docai の Errors 表の1行)。
// Body は共通エラー形から逸脱する場合や field-level エラーのとき、個別 example/表の
// ソースになる(nil のときは表の1行のみ)。
type ErrorSpec struct {
	Status       int     `json:"status"`
	Code         string  `json:"code"`
	Condition    string  `json:"condition,omitempty"`
	CallerAction string  `json:"caller_action,omitempty"`
	Body         *Schema `json:"body,omitempty"`
}

// Behavior は docai「振る舞い」節(型から導けない情報)。すべて Endpoint の宣言由来。
type Behavior struct {
	SideEffects   []string `json:"side_effects,omitempty"`
	Idempotent    *bool    `json:"idempotent,omitempty"` // nil = 未宣言(レンダラで TODO)
	Preconditions []string `json:"preconditions,omitempty"`
}

// Security は誰がエンドポイントを呼べるか。OpenAPI の `security` に 1:1 で写せる。
type Security struct {
	// Public は認証不要。これが唯一エンドポイントを開く手段。
	Public bool `json:"public,omitempty"`
	// Schemes は受け入れる資格情報の種別(空 = 何でも可)。
	Schemes []string `json:"schemes,omitempty"`
	// Scopes は呼び出し元が**すべて**持つ必要のある権限。
	Scopes []string `json:"scopes,omitempty"`
}

// SchemaType は JSON の値種別に対応する中立なスキーマ種別。
// docai レンダラは自前の型名(int/float/bool/object[] 等)へ、OpenAPI レンダラは
// JSON Schema の `type` へ、それぞれ写す。
type SchemaType string

const (
	TypeString  SchemaType = "string"
	TypeInteger SchemaType = "integer"
	TypeNumber  SchemaType = "number"
	TypeBoolean SchemaType = "boolean"
	TypeObject  SchemaType = "object"
	TypeArray   SchemaType = "array"
)

// FormatDateTime は time.Time に対応する Format 値(JSON Schema / OpenAPI の date-time)。
const FormatDateTime = "date-time"

// Schema はボディの型を表す再帰的なスキーマ木。
//
// 木のまま持つことで OpenAPI(ネストした JSON Schema)へ無損失で変換でき、
// docai の平坦なフィールド表へは木を辿って平坦化すればよい(復元不要な方向)。
type Schema struct {
	Type   SchemaType `json:"type,omitempty"`   // 空 = 任意の型(interface{} 等)
	Format string     `json:"format,omitempty"` // date-time 等。空=なし
	// GoType は object ノードの Go 型名(例 "dto.User")。無名構造体では空。
	// OpenAPI の components/$ref に名前を付けるための素材。
	GoType string `json:"go_type,omitempty"`
	// PkgPath は GoType のパッケージ import パス。基底名が同じパッケージ(複数の dto)が
	// あると GoType だけでは一意にならないため、衝突の判別に使う。
	PkgPath     string       `json:"pkg_path,omitempty"`
	Nullable    bool         `json:"nullable,omitempty"` // ポインタ由来(null を取り得る)
	Constraints []Constraint `json:"constraints,omitempty"`
	// Fields は Type==object のプロパティ(宣言順)。動的キーの object では空。
	Fields []*Field `json:"fields,omitempty"`
	// Items は Type==array の要素スキーマ。
	Items *Schema `json:"items,omitempty"`
	// Values は動的キー object(Go の map)の値スキーマ。
	Values *Schema `json:"values,omitempty"`
	// Example は JSON 形の具体例(map/slice/scalar)。決定的・制約充足。
	// ボディのルートノードにのみ設定する。
	Example any `json:"example,omitempty"`
}

// Field は object ノードの1プロパティ。名前と「スロット」の性質(必須/省略可/意味)を持ち、
// 型そのものは Schema が持つ。
type Field struct {
	Name     string  `json:"name"`
	Required bool    `json:"required,omitempty"`
	Optional bool    `json:"optional,omitempty"` // json:",omitempty" タグ由来(省略可能)
	Meaning  string  `json:"meaning,omitempty"`  // FieldDocs 由来。未宣言は空(レンダラで TODO)
	Schema   *Schema `json:"schema"`
}

// Constraint は1つの制約を**構造化して**保持する(出力フォーマット中立)。
// 文言化はレンダラが出力言語に応じて行い(docai: "max length 10" / "最大10文字")、
// OpenAPI レンダラは Code→キーワード(str_max_length→maxLength: params.max)へ変換できる。
//
//   - Code は検証ルールの種別(rule.Descriptor.Name。例 "str_max_length")。
//   - Params は制約パラメータ(例 {"max": 10}、enum は {"values": [...]})。
type Constraint struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// PathParam はパスパラメータ(Module.Path の `{name}`)。制約は PathRules 由来。
type PathParam struct {
	Name        string       `json:"name"`
	Required    bool         `json:"required,omitempty"`
	Constraints []Constraint `json:"constraints,omitempty"`
}

// QueryParam はクエリパラメータ(QueryRules 由来。フィールド名がパラメータ名)。
type QueryParam struct {
	Name        string       `json:"name"`
	Required    bool         `json:"required,omitempty"`
	Constraints []Constraint `json:"constraints,omitempty"`
}

// ResponseHeader は呼び出し側が読むべきレスポンスヘッダ。
type ResponseHeader struct {
	Name    string `json:"name"`
	Meaning string `json:"meaning,omitempty"`
}
