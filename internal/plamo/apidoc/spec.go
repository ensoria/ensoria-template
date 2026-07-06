// Package apidoc は、restkit.Endpoint(型・検証ルール・ルーティング宣言)から
// 出力フォーマット中立の API モデル(APISpec)をリフレクションで組み立てる。
//
// APISpec は docai 専用語彙にせず、HTTP API の一般的な意味論で定義する。
// describe がこれを JSON で出力し、encli(レンダラ)が docai/OpenAPI などへ変換する。
package apidoc

// APISpec は1つの API 全体の中立モデル。
type APISpec struct {
	Endpoints   []*EndpointSpec `json:"endpoints"`
	Conventions *Conventions    `json:"conventions,omitempty"`
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
	Errors            []ErrorSpec      `json:"errors,omitempty"` // エンドポイント固有エラー(§4.1)
	Behavior          Behavior         `json:"behavior"`
	// Untyped は Documented を実装しない生 Controller(型不明)のとき true。
	Untyped bool `json:"untyped,omitempty"`
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
	Scopes        []string `json:"scopes,omitempty"` // Authorization(認可スコープ)
}

// Schema はボディの型を平坦化したフィールド一覧と、具体例(example)。
type Schema struct {
	Fields []Field `json:"fields"`
	// Example は JSON 形の具体例(map/slice/scalar)。決定的・制約充足。
	Example any `json:"example,omitempty"`
}

// Field はスキーマ表の1行(ネスト/配列はドット・角括弧記法で平坦化)。
type Field struct {
	Name        string       `json:"name"` // "address.city" / "items[].id" 等
	Type        string       `json:"type"` // string/int/float/bool/string[]/object/object[] 等
	Required    bool         `json:"required"`
	Nullable    bool         `json:"nullable"`
	Optional    bool         `json:"optional,omitempty"` // json:",omitempty" タグ由来(省略可能)
	Constraints []Constraint `json:"constraints,omitempty"`
	Meaning     string       `json:"meaning,omitempty"` // FieldDocs 由来。未宣言は空(レンダラで TODO)
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
