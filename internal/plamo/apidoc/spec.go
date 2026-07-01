// Package apidoc は、restkit.Endpoint(型・検証ルール・ルーティング宣言)から
// 出力フォーマット中立の API モデル(APISpec)をリフレクションで組み立てる。
//
// APISpec は docai 専用語彙にせず、HTTP API の一般的な意味論で定義する。
// describe がこれを JSON で出力し、encli(レンダラ)が docai/OpenAPI などへ変換する。
package apidoc

// APISpec は1つの API 全体の中立モデル。
type APISpec struct {
	Endpoints []*EndpointSpec `json:"endpoints"`
	// Conventions は Phase 5 で追加(CORS/認証/共通エラー等)。
}

// EndpointSpec は1エンドポイントの中立モデル。
type EndpointSpec struct {
	Method          string           `json:"method"`
	Path            string           `json:"path"`
	Summary         string           `json:"summary,omitempty"`
	Description     string           `json:"description,omitempty"`
	PathParams      []PathParam      `json:"path_params,omitempty"`
	SuccessStatus   int              `json:"success_status,omitempty"`
	Request         *Schema          `json:"request,omitempty"`
	Response        *Schema          `json:"response,omitempty"`
	ResponseHeaders []ResponseHeader `json:"response_headers,omitempty"`
	// Untyped は Documented を実装しない生 Controller(型不明)のとき true。
	Untyped bool `json:"untyped,omitempty"`
}

// Schema はボディの型を平坦化したフィールド一覧。
type Schema struct {
	Fields []Field `json:"fields"`
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

// PathParam はパスパラメータ(Module.Path の `{name}`)。
type PathParam struct {
	Name string `json:"name"`
}

// ResponseHeader は呼び出し側が読むべきレスポンスヘッダ。
type ResponseHeader struct {
	Name    string `json:"name"`
	Meaning string `json:"meaning,omitempty"`
}
