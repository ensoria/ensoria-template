// Package restkit は、型付き HTTP エンドポイント(Endpoint[Req,Res])と、それを
// rest.Controller に適合させるアダプタを提供するフレームワーク・グルー。
//
// 目的は「実装から docai を生成する」ための宣言面を作ること:
// リクエスト/レスポンスの型・検証ルール・ステータス・振る舞いなどを、命令的な
// Handle の内側ではなく Endpoint の宣言フィールドに引き上げる。これにより後段の
// apidoc がリフレクションでドキュメントを組み立てられる。
package restkit

import (
	"reflect"

	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// Endpoint は1つの HTTP エンドポイントの型付き定義。
//
// Req はリクエストボディの型、Res は成功時レスポンスボディの型。ボディの型を
// 型パラメータに固定することで、宣言したスキーマと実際のボディの乖離をコンパイル時に防ぐ。
type Endpoint[Req any, Res any] struct {
	// --- 意味的散文(型から導けないので宣言で持つ。再生成で消えない) ---
	Summary     string            // INDEX 概要 / 見出し直後の1文
	Description string            // 追加の説明
	FieldDocs   map[string]string // フィールド意味(ドット記法キー: "address.city" 等)

	// IDPrefix は example 生成で、このリソースの id に使うプレフィックスを固定する
	// (例 "usr")。空の場合はパス/フィールド名から自動導出する(単数形フルネーム)。
	IDPrefix string

	// --- 検証(適用箇所ごとに分離) ---
	BodyRules  []*rule.RuleSet
	PathRules  []*rule.RuleSet
	QueryRules []*rule.RuleSet

	// --- レスポンス ---
	Success         int          // 主たる成功ステータス(0 の場合は 200)
	ResponseHeaders []HeaderSpec // docai の Response Headers 表の宣言ソース
	Produces        string       // 出力形式を固定する場合のメディアタイプ(空=ネゴシエーション)
	Responses       []ResponseSpec

	// --- エラー ---
	Errors []ErrorSpec

	// --- 振る舞い(型から導けない) ---
	Behavior BehaviorSpec

	// Handle は検証済みのリクエストボディを受け取り、型付きの Result かエラーを返す。
	Handle func(r *rest.Request, req *Req) (*rest.Result[Res], error)
}

// HeaderSpec は docai の `#### Response Headers` 表の1行を宣言する。
type HeaderSpec struct {
	Name    string
	Meaning string
}

// ErrorSpec はこのエンドポイント固有のエラーを宣言する(docai の Errors 表)。
type ErrorSpec struct {
	Status       int
	Code         string
	Condition    string
	CallerAction string
	Retryable    bool
	FieldLevel   bool
	// BodyType はエラーレスポンス本文の型(example 生成に使う)。nil の場合は既定形。
	BodyType reflect.Type
}

// BehaviorSpec は docai の「振る舞い」節(型から導けない情報)を宣言する。
type BehaviorSpec struct {
	SideEffects   []string
	Idempotent    *bool // nil=未宣言(docai に TODO)
	Preconditions []string
	Scopes        []string
}

// ResponseSpec は主レスポンス(Success + Res)以外の成功レスポンスを宣言する
// (200/201 併存や 202 + 別ボディ型など、型からは導けないもの)。
type ResponseSpec struct {
	Status   int
	BodyType reflect.Type
	Headers  []HeaderSpec
	When     string // 発生条件(docai に出す)
}

// EndpointDoc は apidoc が読むための、型情報 + 宣言メタの正規化ビュー。
// Endpoint はジェネリックで直接リフレクションしづらいため、アダプタがこの
// 非ジェネリックな構造体に変換して公開する。
type EndpointDoc struct {
	Summary     string
	Description string
	FieldDocs   map[string]string
	IDPrefix    string

	ReqType reflect.Type
	ResType reflect.Type

	BodyRules  []*rule.RuleSet
	PathRules  []*rule.RuleSet
	QueryRules []*rule.RuleSet

	Success         int
	ResponseHeaders []HeaderSpec
	Produces        string
	Responses       []ResponseSpec
	Errors          []ErrorSpec
	Behavior        BehaviorSpec
}

// Documented はドキュメント用メタを公開するアダプタが満たすインターフェース。
// apidoc は rest.Controller をこのインターフェースに型アサートしてメタを取り出す。
type Documented interface {
	EndpointDoc() EndpointDoc
}
