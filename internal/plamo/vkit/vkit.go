package vkit

import (
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
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

// 数値フィールドの下限・上限。
//
// JSON では数値フィールドの「未指定」と 0 を区別できない(ポインタでない限り、
// 欠けたキーはゼロ値になる)ため、数値に Required は使えない。実質的な必須は
// MinValue(1) のように「取り得ない値を弾く」形で表す。
// TODO: 数値の必須をドキュメントの Required 列にも反映するには、validator 側に
// 存在確認のルールが要る(現状は制約 minimum として出る)。
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
