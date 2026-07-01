package apidoc

import (
	"fmt"
	"strings"

	"github.com/ensoria/validator/pkg/rule"
)

// requiredCodes は「そのフィールドが必須」を意味するルールの Descriptor.Name。
// 制約欄ではなく Required 列に反映する。
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
	if requiredCodes[d.Name] {
		f.Required = true
		return
	}
	if c := constraintText(d); c != "" {
		f.Constraints = append(f.Constraints, c)
	}
}

// constraintText は rule.Descriptor を docai 制約欄の文言に変換する。
func constraintText(d rule.Descriptor) string {
	switch d.Name {
	case "str_max_length":
		return "max length " + param(d, "max")
	case "str_min_length":
		return "min length " + param(d, "min")
	case "str_length_between":
		return "length " + param(d, "min") + "–" + param(d, "max")
	case "str_any_of", "slice_elements_in":
		return "one of: " + valuesList(d)
	case "str_email":
		return "email (RFC 5322)"
	case "str_alpha":
		return "alphabetic"
	case "str_alpha_dash":
		return "alphanumeric or dash"
	case "str_alpha_num":
		return "alphanumeric"
	case "str_url":
		return "URL"
	case "str_uuid_v4":
		return "UUID v4"
	case "str_uuid_v7":
		return "UUID v7"
	case "str_ulid":
		return "ULID"
	case "str_match_regexp":
		return "matches a pattern"
	case "int_min", "float_min":
		return "min " + param(d, "limit")
	case "int_max", "float_max":
		return "max " + param(d, "limit")
	case "int_between", "float_between":
		return param(d, "min") + "–" + param(d, "max")
	case "slice_length_min":
		return "min " + param(d, "length") + " items"
	case "slice_length_max":
		return "max " + param(d, "length") + " items"
	case "slice_length_is":
		return "exactly " + param(d, "length") + " items"
	case "slice_length_between":
		return param(d, "min") + "–" + param(d, "max") + " items"
	case "str_time_after":
		return "after field " + param(d, "field")
	case "str_time_before":
		return "before field " + param(d, "field")
	case "str_time_after_or_equal":
		return "at or after field " + param(d, "field")
	case "str_time_before_or_equal":
		return "at or before field " + param(d, "field")
	default:
		// 未知のルールは code をそのまま出す(取りこぼしを可視化)
		return d.Name
	}
}

// param は Descriptor.Params[key] を文字列化する。無ければ "?"。
func param(d rule.Descriptor, key string) string {
	if v, ok := d.Params[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return "?"
}

// valuesList は enum(Params["values"])をカンマ区切りにする。
func valuesList(d rule.Descriptor) string {
	values, ok := d.Params["values"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%v", v)
	}
	return strings.Join(parts, ", ")
}
