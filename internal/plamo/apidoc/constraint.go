package apidoc

import (
	"github.com/ensoria/validator/pkg/rule"
)

// requiredCodes は「そのフィールドが必須」を意味するルールの Descriptor.Name。
// 制約(Constraints)ではなく Required 列に反映する。
//
// not_nil はポインタ型フィールド向けで、型を問わず「指定されたか」を見る。
// num_not_zero は 0 を「未指定」とみなす妥協手段(0 が正当な値のフィールドには使えない)。
var requiredCodes = map[string]bool{
	"str_not_empty":   true,
	"slice_not_empty": true,
	"not_nil":         true,
	"num_not_zero":    true,
}

// applyRules は ruleSets の制約をスキーマ木の該当フィールドに反映する。
// RuleSet.Field はドット/角括弧記法のパス("address.city" / "items[].id")でネストを指す。
//
// 必須フラグはフィールド(スロット)に、その他の制約は型を表すスキーマノードに載せる
// (JSON Schema と同じ置き場所)。
func applyRules(schema *Schema, ruleSets []*rule.RuleSet) {
	if schema == nil {
		return
	}
	for _, rs := range ruleSets {
		f := findField(schema, rs.Field)
		if f == nil {
			continue // スキーマに無いフィールド(パス/クエリ等)はここでは対象外
		}
		for _, r := range rs.Rules {
			applyRuleDescriptor(&f.Required, &f.Schema.Constraints, r.Descriptor)
			applyNullability(f.Schema, r.Descriptor)
		}
		for _, fcr := range rs.FieldCompareRules {
			applyRuleDescriptor(&f.Required, &f.Schema.Constraints, fcr.Descriptor)
		}
	}
}

// applyNullability は宣言されたルールをスキーマの Nullable に反映する。
//
// Nullable は Go の型から導いている(ポインタ = null を取り得る)が、not_nil を宣言した
// フィールドは null を拒否する。型だけを見ると「必須なのに null 可」という、実装と
// 食い違うドキュメントになるため、宣言のほうを優先する。
func applyNullability(schema *Schema, d rule.Descriptor) {
	if schema != nil && d.Name == notNilCode {
		schema.Nullable = false
	}
}

// notNilCode は「null を許さない」ことを意味するルールの Descriptor.Name。
const notNilCode = "not_nil"

// applyRuleDescriptor は「必須」系はフラグに、それ以外は構造化 Constraint に反映する
// 共通ロジック(フィールド/パス/クエリで共有。文言化はレンダラ側)。
func applyRuleDescriptor(required *bool, constraints *[]Constraint, d rule.Descriptor) {
	if requiredCodes[d.Name] {
		*required = true
		return
	}
	*constraints = append(*constraints, Constraint{Code: d.Name, Params: d.Params})
}

// descriptorsByField は RuleSet 群をフィールド名 → 記述子一覧に索引化する。
func descriptorsByField(ruleSets []*rule.RuleSet) map[string][]rule.Descriptor {
	m := map[string][]rule.Descriptor{}
	for _, rs := range ruleSets {
		for _, r := range rs.Rules {
			m[rs.Field] = append(m[rs.Field], r.Descriptor)
		}
		for _, fcr := range rs.FieldCompareRules {
			m[rs.Field] = append(m[rs.Field], fcr.Descriptor)
		}
	}
	return m
}
