package vkit

import (
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/validator/pkg/validate"
	"github.com/ensoria/validator/pkg/verr"
)

// RestRequestBody は rest.Request のボディを T にパースして検証する。
// 検証違反はプロトコル非依存の中立形 verr.ValidationErrors(全言語 + code 付き)で返す。
func RestRequestBody[T any](r *rest.Request, ruleSets ...*rule.RuleSet) (*T, verr.ValidationErrors) {
	return validate.RestRequestBody[T](r, ruleSets...)
}

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

// TODO: 他のバリデーションも一通り定義する
