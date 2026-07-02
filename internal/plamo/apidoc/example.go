package apidoc

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/ensoria/gofake/pkg/faker"
	"github.com/ensoria/validator/pkg/rule"
)

// exampleSeed は example 生成に使う固定シード。golden テストのため決定的にする。
const exampleSeed = 20260611

// 横断的に一貫させたいフィールドの固定値(README §4.1: 同一 ID/日時を使い回す)。
const (
	idSuffix    = "01HXYZ7A8B9C0D1E2F3G" // id の接尾辞(プレフィックスはリソースごと)
	fixtureTime = "2026-06-11T09:30:00Z"
)

// ExampleOptions は example 生成のリソース文脈。
type ExampleOptions struct {
	// Resource はこのエンドポイントのリソース名(単数形)。bare な `id` に使う。
	Resource string
	// IDPrefixes はリソース名 → 宣言された id プレフィックスの対応(上書き用)。
	// 未宣言のリソースはリソース名(単数形フルネーム)を自動プレフィックスにする。
	IDPrefixes map[string]string
}

// ExampleFromType は t に対応する JSON 形の example(map/slice/scalar)を、
// ruleSets の制約を満たすように生成する。固定シードにより決定的。
func ExampleFromType(t reflect.Type, ruleSets []*rule.RuleSet, opts ExampleOptions) any {
	if t == nil {
		return nil
	}
	g := &exampleGen{
		faker:       faker.CreateWithSeed(exampleSeed),
		constraints: constraintsByField(ruleSets),
		opts:        opts,
	}
	return g.typeValue(t, "")
}

type exampleGen struct {
	faker       *faker.Faker
	constraints map[string][]rule.Descriptor
	opts        ExampleOptions
}

// constraintsByField は RuleSet 群をフィールド名 → 記述子一覧に索引化する。
func constraintsByField(ruleSets []*rule.RuleSet) map[string][]rule.Descriptor {
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

// typeValue は型を JSON 形の値に変換する(ポインタは剥がす。スライスは要素1つ)。
func (g *exampleGen) typeValue(t reflect.Type, path string) any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return fixtureTime
	case t.Kind() == reflect.Struct:
		return g.structValue(t, path)
	case t.Kind() == reflect.Slice || t.Kind() == reflect.Array:
		return []any{g.typeValue(t.Elem(), path+"[]")}
	case t.Kind() == reflect.Map:
		return map[string]any{}
	default:
		return g.scalarValue(t, path)
	}
}

// structValue は構造体を map[string]any にする(json タグ名をキーに)。
func (g *exampleGen) structValue(t reflect.Type, prefix string) map[string]any {
	m := map[string]any{}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name := jsonName(sf)
		if name == "-" {
			continue
		}
		full := name
		if prefix != "" {
			full = prefix + "." + name
		}
		m[name] = g.typeValue(sf.Type, full)
	}
	return m
}

// scalarValue はプリミティブ値を生成する(制約・フィールド名ヒューリスティックを反映)。
func (g *exampleGen) scalarValue(t reflect.Type, path string) any {
	ds := g.constraints[path]
	switch t.Kind() {
	case reflect.String:
		return g.stringValue(path, ds)
	case reflect.Bool:
		return g.faker.Rand.Bool.Evenly()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return g.intValue(ds)
	case reflect.Float32, reflect.Float64:
		return g.floatValue(ds)
	default:
		return nil
	}
}

// stringValue は文字列値を、enum → フィールド名ヒューリスティック → 汎用語 の順で決め、
// 最後に長さ制約(max/min)を適用する。
func (g *exampleGen) stringValue(path string, ds []rule.Descriptor) string {
	if v, ok := enumFirst(ds); ok {
		return v
	}

	leaf := leafName(path)
	var s string
	switch {
	case strings.Contains(leaf, "email"):
		s = g.faker.Internet.Email()
	case leaf == "id" || strings.HasSuffix(leaf, "_id") ||
		strings.Contains(leaf, "uuid") || strings.Contains(leaf, "ulid"):
		s = g.idValue(leaf)
	case strings.Contains(leaf, "name"):
		s = g.faker.Person.Name()
	case strings.Contains(leaf, "url"):
		s = g.faker.Internet.URL()
	case strings.HasSuffix(leaf, "_at") || strings.Contains(leaf, "time") || strings.Contains(leaf, "date"):
		s = fixtureTime
	default:
		s = g.faker.Lorem.Word()
	}

	return applyLength(s, ds)
}

// intValue は制約(min/max/between)を満たす整数を生成する。
func (g *exampleGen) intValue(ds []rule.Descriptor) int {
	lo, hi := 1, 1000
	if v, ok := intParam(ds, "int_min", "limit"); ok {
		lo = v
	}
	if v, ok := intParam(ds, "int_max", "limit"); ok {
		hi = v
	}
	if v, ok := intParam(ds, "int_between", "min"); ok {
		lo = v
	}
	if v, ok := intParam(ds, "int_between", "max"); ok {
		hi = v
	}
	if lo > hi {
		hi = lo
	}
	return g.faker.Rand.Num.IntBt(lo, hi)
}

// floatValue は制約(min/max/between)を満たす浮動小数を生成する。
func (g *exampleGen) floatValue(ds []rule.Descriptor) float64 {
	lo, hi := 1.0, 1000.0
	if v, ok := floatParam(ds, "float_min", "limit"); ok {
		lo = v
	}
	if v, ok := floatParam(ds, "float_max", "limit"); ok {
		hi = v
	}
	if v, ok := floatParam(ds, "float_between", "min"); ok {
		lo = v
	}
	if v, ok := floatParam(ds, "float_between", "max"); ok {
		hi = v
	}
	if lo > hi {
		hi = lo
	}
	return g.faker.Rand.Num.Float64Bt(lo, hi)
}

// idValue は id 系フィールドの値を「プレフィックス_接尾辞」で組み立てる。
// `<name>_id` はフィールド名から、bare な `id`/uuid/ulid はエンドポイントのリソースから
// プレフィックスを決める。
func (g *exampleGen) idValue(leaf string) string {
	resource := g.opts.Resource
	if strings.HasSuffix(leaf, "_id") {
		resource = singular(strings.TrimSuffix(leaf, "_id"))
	}
	return g.prefixFor(resource) + "_" + idSuffix
}

// prefixFor はリソース名から id プレフィックスを返す。
// 宣言(IDPrefixes)があればそれ、無ければリソース名(単数形フルネーム)。
func (g *exampleGen) prefixFor(resource string) string {
	if resource == "" {
		return "id"
	}
	if p, ok := g.opts.IDPrefixes[resource]; ok && p != "" {
		return p
	}
	return resource
}

// --- ヘルパー ---

// leafName は "items[].address.city" のようなパスから末尾フィールド名("city")を返す。
func leafName(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		path = path[i+1:]
	}
	return strings.TrimSuffix(path, "[]")
}

// enumFirst は str_any_of / slice_elements_in の最初の許可値を返す。
func enumFirst(ds []rule.Descriptor) (string, bool) {
	for _, d := range ds {
		if d.Name != "str_any_of" && d.Name != "slice_elements_in" {
			continue
		}
		if values, ok := d.Params["values"].([]any); ok && len(values) > 0 {
			return fmt.Sprintf("%v", values[0]), true
		}
	}
	return "", false
}

// applyLength は max/min 長制約を満たすよう文字列を調整する。
func applyLength(s string, ds []rule.Descriptor) string {
	if v, ok := intParam(ds, "str_max_length", "max"); ok && len(s) > v {
		s = s[:v]
	}
	if v, ok := intParam(ds, "str_length_between", "max"); ok && len(s) > v {
		s = s[:v]
	}
	min := 0
	if v, ok := intParam(ds, "str_min_length", "min"); ok {
		min = v
	}
	if v, ok := intParam(ds, "str_length_between", "min"); ok && v > min {
		min = v
	}
	for len(s) < min {
		s += "x"
	}
	return s
}

// intParam は ds から code のルールを探し、key の整数パラメータを返す。
func intParam(ds []rule.Descriptor, code, key string) (int, bool) {
	for _, d := range ds {
		if d.Name != code {
			continue
		}
		if v, ok := d.Params[key].(int); ok {
			return v, true
		}
	}
	return 0, false
}

// floatParam は ds から code のルールを探し、key の数値パラメータを返す(int/float 両対応)。
func floatParam(ds []rule.Descriptor, code, key string) (float64, bool) {
	for _, d := range ds {
		if d.Name != code {
			continue
		}
		switch v := d.Params[key].(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		}
	}
	return 0, false
}
