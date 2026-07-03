package apidoc

import (
	"github.com/ensoria/validator/pkg/rule"
)

// requiredCodes は「そのフィールドが必須」を意味するルールの Descriptor.Name。
// 制約(Constraints)ではなく Required 列に反映する。
var requiredCodes = map[string]bool{
	"str_not_empty":   true,
	"slice_not_empty": true,
}

// applyRules は ruleSets の制約をスキーマの各フィールドに反映する。
// ルールのフィールド名(RuleSet.Field)とスキーマのフィールド名で突き合わせる。
func applyRules(schema *Schema, ruleSets []*rule.RuleSet) {
	if schema == nil {
		return
	}
	byField := make(map[string]*Field, len(schema.Fields))
	for i := range schema.Fields {
		byField[schema.Fields[i].Name] = &schema.Fields[i]
	}
	for _, rs := range ruleSets {
		f, ok := byField[rs.Field]
		if !ok {
			continue // スキーマに無いフィールド(パス/クエリ等)はここでは対象外
		}
		for _, r := range rs.Rules {
			applyDescriptor(f, r.Descriptor)
		}
		for _, fcr := range rs.FieldCompareRules {
			applyDescriptor(f, fcr.Descriptor)
		}
	}
}

// applyDescriptor は1つのルール記述子をフィールドに反映する。
func applyDescriptor(f *Field, d rule.Descriptor) {
	applyRuleDescriptor(&f.Required, &f.Constraints, d)
}

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
