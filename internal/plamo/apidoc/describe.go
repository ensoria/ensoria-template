package apidoc

import (
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// Build は HTTP モジュール群を走査して APISpec を組み立てる。
// 各モジュールの Get/Post/... のうち、Documented を実装するものは型付きスペックに、
// そうでない生 Controller は型不明(Untyped)スペックにする。
func Build(modules []*rest.Module) *APISpec {
	spec := &APISpec{}
	for _, m := range modules {
		spec.Endpoints = append(spec.Endpoints, DescribeModule(m)...)
	}
	return spec
}

// DescribeModule は1モジュールの各メソッドを EndpointSpec に変換する。
func DescribeModule(m *rest.Module) []*EndpointSpec {
	methods := []struct {
		name string
		ctrl rest.Controller
	}{
		{"GET", m.Get},
		{"POST", m.Post},
		{"PUT", m.Put},
		{"PATCH", m.Patch},
		{"DELETE", m.Delete},
	}

	var specs []*EndpointSpec
	for _, mc := range methods {
		if mc.ctrl == nil {
			continue
		}
		if doc, ok := mc.ctrl.(restkit.Documented); ok {
			specs = append(specs, DescribeEndpoint(mc.name, m.Path, doc.EndpointDoc()))
		} else {
			// 生 Controller: 型・宣言メタが無いので method/path のみ(レンダラで TODO)
			specs = append(specs, &EndpointSpec{
				Method:     mc.name,
				Path:       m.Path,
				PathParams: parsePathParams(m.Path),
				Untyped:    true,
			})
		}
	}
	return specs
}

// DescribeEndpoint は method/path と EndpointDoc から EndpointSpec を組み立てる。
func DescribeEndpoint(method, path string, doc restkit.EndpointDoc) *EndpointSpec {
	req := SchemaFromType(doc.ReqType)
	applyRules(req, doc.BodyRules)
	applyFieldDocs(req, doc.FieldDocs)

	res := SchemaFromType(doc.ResType)
	applyFieldDocs(res, doc.FieldDocs)

	return &EndpointSpec{
		Method:          method,
		Path:            path,
		Summary:         doc.Summary,
		Description:     doc.Description,
		PathParams:      parsePathParams(path),
		SuccessStatus:   doc.Success,
		Request:         req,
		Response:        res,
		ResponseHeaders: convertHeaders(doc.ResponseHeaders),
	}
}

// parsePathParams は Path の `{name}` セグメントを抽出する。
func parsePathParams(path string) []PathParam {
	var params []PathParam
	for _, seg := range strings.Split(path, "/") {
		if len(seg) >= 2 && strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			params = append(params, PathParam{Name: seg[1 : len(seg)-1]})
		}
	}
	return params
}

// applyFieldDocs は宣言されたフィールド意味(ドット記法キー)をスキーマに反映する。
func applyFieldDocs(schema *Schema, fieldDocs map[string]string) {
	if schema == nil || len(fieldDocs) == 0 {
		return
	}
	for i := range schema.Fields {
		if meaning, ok := fieldDocs[schema.Fields[i].Name]; ok {
			schema.Fields[i].Meaning = meaning
		}
	}
}

// convertHeaders は restkit.HeaderSpec を apidoc.ResponseHeader へ変換する。
func convertHeaders(headers []restkit.HeaderSpec) []ResponseHeader {
	if len(headers) == 0 {
		return nil
	}
	out := make([]ResponseHeader, len(headers))
	for i, h := range headers {
		out[i] = ResponseHeader{Name: h.Name, Meaning: h.Meaning}
	}
	return out
}
