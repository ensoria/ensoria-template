package vkit

import (
	"encoding/json"

	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/validator/pkg/util"
	"github.com/ensoria/validator/pkg/validate"
	"github.com/ensoria/validator/pkg/verr"
)

// DELETE: ラップする必要がなくなったので削除
// RestRequestBody は rest.Request のボディを T にパースして検証する。
// 検証違反はプロトコル非依存の中立形 verr.ValidationErrors(全言語 + code 付き)で返す。
func RestRequestBody[T any](r *rest.Request, ruleSets ...*rule.RuleSet) (*T, verr.ValidationErrors) {
	return validate.RestRequestBody[T](r, ruleSets...)
}

// DELETE: ラップする必要がなくなったので削除
// Map は Query / Path / Header などの map 値を検証する。
func Map[T any](m map[string]T, ruleSets ...*rule.RuleSet) verr.ValidationErrors {
	return validate.Map(m, ruleSets...)
}

// Object validates a value that has already been decoded.
//
// RestRequestBody covers the HTTP case, where parsing and validation happen
// together. The messaging surfaces (mbkit, wskit) receive raw bytes off a broker
// or a socket instead, so they decode first and validate here — reusing the same
// rule sets and returning the same neutral verr.ValidationErrors, so a field is
// constrained identically no matter which transport it arrived on.
func Object[T any](obj *T, ruleSets ...*rule.RuleSet) verr.ValidationErrors {
	return validate.Map(util.StructToJsonKeyMap(obj), ruleSets...)
}

// JSONBody decodes raw JSON into T and validates the result.
// A decoding failure comes back as verr.ParseError, matching how
// RestRequestBody reports an unparsable HTTP body.
func JSONBody[T any](data []byte, ruleSets ...*rule.RuleSet) (*T, verr.ValidationErrors) {
	var obj T
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, verr.ParseError(err)
	}
	if errs := Object(&obj, ruleSets...); errs.HasErrors() {
		return nil, errs
	}
	return &obj, nil
}

// 以下のバリデーションは、共通で使えるようなメッセージで定義しています。
// より詳細なメッセージでメッセージを定義したい場合は、各Module内で
// 別のメッセージのRuleFactoryの定義を作成してください。

var Required = rule.CreateStrNotEmpty(map[string]string{
	"ja": "必須です",
	"en": "this field is required",
})

var MaxLength = rule.CreateStrMaxLength(
	map[string]string{
		"ja": "最大文字数%dを超えています",
		"en": "exceeds maximum length of %d characters",
	})

// NotNil はフィールドが指定されたことを検証する。型を問わない。
//
// **ポインタ型のフィールドにのみ意味がある。** JSON では欠けたキーがゼロ値になるため、
// 非ポインタのフィールドには常に値があり、このルールは必ず成功する —— 宣言だけが
// 残り、何も強制されない状態になる。ポインタにすれば「0 を指定した」と
// 「指定しなかった」を区別できる。
//
//	// dto: OrderId *int `json:"orderId"`
//	{Field: "orderId", Rules: []rule.Rule{vkit.NotNil()}}
var NotNil = rule.CreateNotNil(map[string]string{
	"ja": "必須です",
	"en": "this field is required",
})

// NotNullIfSet はフィールドが指定された場合に、それが null でないことを検証する。
// 指定されなかった場合は通す。
//
// 部分更新(PATCH)で「触れないのは自由だが、消すことは許さない」フィールドに使う。
// **optional.Optional[T] のフィールドでのみ意味がある** —— 未指定と null を区別できるのは
// この型だけで、他の型では常に成功する。
//
//	// dto: Name optional.Optional[string] `json:"name"`
//	{Field: "name", Rules: []rule.Rule{vkit.NotNullIfSet()}}
var NotNullIfSet = rule.CreateNotNullIfSet(map[string]string{
	"ja": "null にはできません",
	"en": "cannot be cleared",
})

// SliceNotEmpty はスライスが空でないことを検証する。
// 空スライスを「未指定」とみなすので、要素0件を正当な値として受け取りたい場合は
// フィールドをポインタにして NotNil を使うこと。
var SliceNotEmpty = rule.CreateSliceNotEmpty(map[string]string{
	"ja": "1件以上指定してください",
	"en": "must contain at least one item",
})

// NumNotZero は数値フィールドが 0 でないことを検証する。
//
// フィールドをポインタにしたくない場合の妥協手段。**0 を「未指定」と同義に扱う**ため、
// 0 が正当な値のフィールド(件数・残高・座標など)には使えない。
// そういうフィールドを必須にしたい場合は、ポインタにして NotNil を使うこと。
var NumNotZero = rule.CreateNumNotZero(map[string]string{
	"ja": "必須です",
	"en": "this field is required",
})

// 数値フィールドの下限・上限。
var MinValue = rule.CreateIntMin(
	map[string]string{
		"ja": "%d以上である必要があります",
		"en": "must be %d or greater",
	})

var MaxValue = rule.CreateIntMax(
	map[string]string{
		"ja": "%d以下である必要があります",
		"en": "must be %d or less",
	})

// TODO: 他のバリデーションも一通り定義する
