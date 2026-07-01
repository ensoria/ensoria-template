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
// 「必須」系はフラグ、それ以外は構造化した Constraint として蓄える(文言化はレンダラ側)。
func applyDescriptor(f *Field, d rule.Descriptor) {
	if requiredCodes[d.Name] {
		f.Required = true
		return
	}
	f.Constraints = append(f.Constraints, Constraint{Code: d.Name, Params: d.Params})
}
