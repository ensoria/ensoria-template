package apidoc

import (
	"reflect"
	"strings"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

// SchemaFromType は構造体型を docai スタイルの平坦なフィールド一覧に変換する。
// ポインタは剥がして Nullable に、ネスト構造体はドット記法、構造体スライスは `[]` で平坦化する。
// 構造体でない型(またはボディ無し)の場合は nil を返す。
func SchemaFromType(t reflect.Type) *Schema {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == timeType {
		return nil
	}
	var fields []Field
	collectFields(t, "", &fields)
	return &Schema{Fields: fields}
}

// collectFields は t のエクスポートされたフィールドを prefix 付きで out に追加する。
func collectFields(t reflect.Type, prefix string, out *[]Field) {
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

		ft := sf.Type
		nullable := false
		for ft.Kind() == reflect.Pointer {
			nullable = true
			ft = ft.Elem()
		}

		field := Field{Name: full, Nullable: nullable, Optional: hasOmitempty(sf)}

		switch {
		case ft == timeType:
			field.Type = "string (RFC 3339)"
			*out = append(*out, field)
		case ft.Kind() == reflect.Struct:
			field.Type = "object"
			*out = append(*out, field)
			collectFields(ft, full, out)
		case ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array:
			et := ft.Elem()
			for et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct && et != timeType {
				field.Type = "object[]"
				*out = append(*out, field)
				collectFields(et, full+"[]", out)
			} else {
				field.Type = primitiveName(et) + "[]"
				*out = append(*out, field)
			}
		case ft.Kind() == reflect.Map:
			// docai は map を非推奨(ネスト構造体を推奨)だが、来た場合は object 扱い
			field.Type = "object"
			*out = append(*out, field)
		default:
			field.Type = primitiveName(ft)
			*out = append(*out, field)
		}
	}
}

// primitiveName は reflect.Kind を docai の平易な型名へ正規化する。
func primitiveName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Struct:
		return "object"
	default:
		return t.Kind().String()
	}
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
