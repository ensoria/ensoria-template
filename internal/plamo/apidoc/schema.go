package apidoc

import (
	"reflect"
	"strings"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

// SchemaFromType は型を再帰的なスキーマ木に変換する。
// ポインタは剥がして Nullable に、構造体は object(Fields)、スライス/配列は array(Items)、
// map は動的キーの object(Values)にする。time.Time は date-time 形式の文字列。
//
// ボディが無い場合(型が nil、または公開フィールドを持たない構造体 = restkit.NoBody)は nil を返す。
func SchemaFromType(t reflect.Type) *Schema {
	if t == nil {
		return nil
	}
	s := buildSchema(t, nil)
	if s.Type == TypeObject && len(s.Fields) == 0 && s.Values == nil {
		return nil
	}
	return s
}

// buildSchema は型を1つのスキーマノード(必要なら子ノードごと)に変換する。
//
// visiting は「今辿っている経路上の構造体型」の集合。自己参照型で無限再帰しないための
// ガードで、経路から抜けるときに取り除くため、循環でない同一型の再出現は毎回展開される。
func buildSchema(t reflect.Type, visiting map[reflect.Type]bool) *Schema {
	s := &Schema{}
	for t.Kind() == reflect.Pointer {
		s.Nullable = true
		t = t.Elem()
	}

	switch {
	case t == timeType:
		s.Type = TypeString
		s.Format = FormatDateTime
	case t.Kind() == reflect.Struct:
		s.Type = TypeObject
		s.GoType = goTypeName(t)
		s.PkgPath = t.PkgPath()
		if visiting[t] {
			// 循環参照: これ以上展開しない(型名は残すので参照先は分かる)。
			return s
		}
		if visiting == nil {
			visiting = map[reflect.Type]bool{}
		}
		visiting[t] = true
		s.Fields = structFields(t, visiting)
		delete(visiting, t)
	case t.Kind() == reflect.Slice || t.Kind() == reflect.Array:
		s.Type = TypeArray
		s.Items = buildSchema(t.Elem(), visiting)
	case t.Kind() == reflect.Map:
		// 動的キーの object(OpenAPI の additionalProperties)。
		s.Type = TypeObject
		s.Values = buildSchema(t.Elem(), visiting)
	default:
		s.Type = scalarType(t)
	}
	return s
}

// structFields は t の公開フィールドを Field 群に変換する(宣言順)。
func structFields(t reflect.Type, visiting map[reflect.Type]bool) []*Field {
	var fields []*Field
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name := jsonName(sf)
		if name == "-" {
			continue
		}
		fields = append(fields, &Field{
			Name:     name,
			Optional: hasOmitempty(sf),
			Schema:   buildSchema(sf.Type, visiting),
		})
	}
	return fields
}

// scalarType は reflect.Kind を中立なスキーマ種別へ正規化する。
// interface{} など JSON の値種別に固定できない型は空(= 任意の型)にする。
func scalarType(t reflect.Type) SchemaType {
	switch t.Kind() {
	case reflect.String:
		return TypeString
	case reflect.Bool:
		return TypeBoolean
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return TypeInteger
	case reflect.Float32, reflect.Float64:
		return TypeNumber
	default:
		return ""
	}
}

// goTypeName は object ノードに載せる Go 型名(例 "dto.User")を返す。無名構造体では空。
func goTypeName(t reflect.Type) string {
	if t.Name() == "" {
		return ""
	}
	return t.String()
}

// findField はドット/角括弧記法のパス("address.city" / "items[].id")でスキーマ木を辿り、
// 該当するフィールドを返す。見つからなければ nil。
// 宣言(BodyRules の RuleSet.Field / FieldDocs のキー)はこの記法でネストを指す。
func findField(schema *Schema, path string) *Field {
	if schema == nil || path == "" {
		return nil
	}
	cur, segs := schema, strings.Split(path, ".")
	for i, seg := range segs {
		name, arrays := splitArraySuffix(seg)
		f := fieldNamed(cur, name)
		if f == nil {
			return nil
		}
		if i == len(segs)-1 {
			return f
		}
		cur = f.Schema
		for ; arrays > 0; arrays-- {
			if cur == nil || cur.Items == nil {
				return nil
			}
			cur = cur.Items
		}
	}
	return nil
}

// splitArraySuffix は末尾の `[]` を数えて分解する("items[][]" → "items", 2)。
func splitArraySuffix(seg string) (string, int) {
	n := 0
	for strings.HasSuffix(seg, "[]") {
		seg = strings.TrimSuffix(seg, "[]")
		n++
	}
	return seg, n
}

// fieldNamed は object ノードから名前でフィールドを引く。
func fieldNamed(s *Schema, name string) *Field {
	if s == nil {
		return nil
	}
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// jsonName は json タグからフィールド名を取り出す。タグが無ければフィールド名。
func jsonName(sf reflect.StructField) string {
	tag := sf.Tag.Get("json")
	if tag == "" {
		return sf.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return sf.Name
	}
	return name
}

// hasOmitempty は json タグに omitempty オプションがあるかを返す。
func hasOmitempty(sf reflect.StructField) bool {
	tag := sf.Tag.Get("json")
	_, opts, _ := strings.Cut(tag, ",")
	for _, opt := range strings.Split(opts, ",") {
		if opt == "omitempty" {
			return true
		}
	}
	return false
}
