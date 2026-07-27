package apidoc

import (
	"reflect"
	"sort"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// Build は HTTP モジュール群を走査して APISpec を組み立てる。
// 各モジュールの Get/Post/... のうち、Documented を実装するものは型付きスペックに、
// そうでない生 Controller は型不明(Untyped)スペックにする。
//
// 先に全エンドポイントを走査して「リソース → 宣言 id プレフィックス」を集め、
// example 生成でリソースをまたいで一貫した id を出せるようにする。
//
// DI はモジュール group を任意の順で解決するため、最後に path→method で安定ソートして
// 出力を決定的にする(そうしないと中身が同じでも spec の JSON が実行ごとに入れ替わる)。
func Build(modules []*rest.Module) *APISpec {
	idPrefixes := collectIDPrefixes(modules)
	spec := &APISpec{}
	for _, m := range modules {
		spec.Endpoints = append(spec.Endpoints, DescribeModule(m, idPrefixes)...)
	}
	sortEndpoints(spec.Endpoints)
	return spec
}

// sortEndpoints は path→method の昇順に並べ替える(決定的な出力のため)。
func sortEndpoints(endpoints []*EndpointSpec) {
	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].Method < endpoints[j].Method
	})
}

// collectIDPrefixes は各エンドポイントの宣言(EndpointDoc.IDPrefix)を
// リソース名(パス第1セグメントの単数形)→ プレフィックス に集約する。
func collectIDPrefixes(modules []*rest.Module) map[string]string {
	prefixes := map[string]string{}
	for _, m := range modules {
		resource := resourceOf(m.Path)
		if resource == "" {
			continue
		}
		for _, ctrl := range []rest.Controller{m.Get, m.Post, m.Put, m.Patch, m.Delete} {
			if doc, ok := ctrl.(restkit.Documented); ok {
				if p := doc.EndpointDoc().IDPrefix; p != "" {
					prefixes[resource] = p
				}
			}
		}
	}
	return prefixes
}

// DescribeModule は1モジュールの各メソッドを EndpointSpec に変換する。
// idPrefixes は example の id プレフィックス解決に使う(nil 可)。
func DescribeModule(m *rest.Module, idPrefixes map[string]string) []*EndpointSpec {
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
			specs = append(specs, DescribeEndpoint(mc.name, m.Path, doc.EndpointDoc(), idPrefixes))
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
func DescribeEndpoint(method, path string, doc restkit.EndpointDoc, idPrefixes map[string]string) *EndpointSpec {
	opts := ExampleOptions{Resource: resourceOf(path), IDPrefixes: idPrefixes}

	req := SchemaFromType(doc.ReqType)
	applyRules(req, doc.BodyRules)
	applyFieldDocs(req, doc.FieldDocs)
	if req != nil {
		req.Example = ExampleFromType(doc.ReqType, doc.BodyRules, opts)
	}

	res := SchemaFromType(doc.ResType)
	applyFieldDocs(res, doc.FieldDocs)
	if res != nil {
		res.Example = ExampleFromType(doc.ResType, nil, opts)
	}

	return &EndpointSpec{
		Method:            method,
		Path:              path,
		Summary:           doc.Summary,
		Description:       doc.Description,
		Task:              doc.Task,
		AlsoRead:          doc.AlsoRead,
		Related:           doc.Related,
		PathParams:        buildPathParams(path, doc.PathRules),
		QueryParams:       buildQueryParams(doc.QueryRules),
		SuccessStatus:     doc.Success,
		Request:           req,
		Response:          res,
		ResponseMediaType: doc.Produces,
		ResponseHeaders:   convertHeaders(doc.ResponseHeaders),
		Responses:         convertResponses(doc.Responses, opts),
		Errors:            convertErrors(doc.Errors, opts),
		Behavior:          convertBehavior(doc.Behavior),
	}
}

// convertResponses は主レスポンス以外の成功レスポンスを写す。
// BodyType が宣言されていれば、その型からスキーマ + example を組む(nil なら本文なし)。
func convertResponses(responses []restkit.ResponseSpec, opts ExampleOptions) []ResponseSpec {
	if len(responses) == 0 {
		return nil
	}
	out := make([]ResponseSpec, 0, len(responses))
	for _, r := range responses {
		spec := ResponseSpec{
			Status:  r.Status,
			When:    r.When,
			Headers: convertHeaders(r.Headers),
		}
		if r.BodyType != nil {
			if body := SchemaFromType(r.BodyType); body != nil {
				body.Example = ExampleFromType(r.BodyType, nil, opts)
				spec.Body = body
			}
		}
		out = append(out, spec)
	}
	return out
}

// convertErrors は restkit.ErrorSpec を apidoc.ErrorSpec へ写す。
// BodyType が宣言されているエラーは、その型から個別 example + フィールド表を組む
// (共通エラー形から逸脱する場合や field-level エラー用。§4.1)。
func convertErrors(errs []restkit.ErrorSpec, opts ExampleOptions) []ErrorSpec {
	if len(errs) == 0 {
		return nil
	}
	out := make([]ErrorSpec, 0, len(errs))
	for _, e := range errs {
		spec := ErrorSpec{
			Status:       e.Status,
			Code:         e.Code,
			Condition:    e.Condition,
			CallerAction: e.CallerAction,
		}
		if e.BodyType != nil {
			if body := SchemaFromType(e.BodyType); body != nil {
				body.Example = errorExample(e.BodyType, e.Code, opts)
				spec.Body = body
			}
		}
		out = append(out, spec)
	}
	return out
}

// errorExample はエラー本文の example を型から生成し、エラーエンベロープの
// `error.code` を宣言された Code に差し替える(faker 語ではなく実際のコードを見せる)。
func errorExample(t reflect.Type, code string, opts ExampleOptions) any {
	ex := ExampleFromType(t, nil, opts)
	if code == "" {
		return ex
	}
	if envelope, ok := ex.(map[string]any); ok {
		if detail, ok := envelope["error"].(map[string]any); ok {
			if _, has := detail["code"]; has {
				detail["code"] = code
			}
		}
	}
	return ex
}

// buildPathParams はパスの `{name}` を抽出し、PathRules の制約を各パラメータに反映する。
func buildPathParams(path string, rules []*rule.RuleSet) []PathParam {
	byField := descriptorsByField(rules)
	var out []PathParam
	for _, name := range parsePathParamNames(path) {
		p := PathParam{Name: name}
		for _, d := range byField[name] {
			applyRuleDescriptor(&p.Required, &p.Constraints, d)
		}
		out = append(out, p)
	}
	return out
}

// buildQueryParams は QueryRules から(フィールド名=パラメータ名で)クエリパラメータを組む。
func buildQueryParams(rules []*rule.RuleSet) []QueryParam {
	var out []QueryParam
	for _, rs := range rules {
		q := QueryParam{Name: rs.Field}
		for _, r := range rs.Rules {
			applyRuleDescriptor(&q.Required, &q.Constraints, r.Descriptor)
		}
		for _, fcr := range rs.FieldCompareRules {
			applyRuleDescriptor(&q.Required, &q.Constraints, fcr.Descriptor)
		}
		out = append(out, q)
	}
	return out
}

// convertBehavior は restkit.BehaviorSpec を apidoc.Behavior へ写す。
func convertBehavior(b restkit.BehaviorSpec) Behavior {
	return Behavior{
		SideEffects:   b.SideEffects,
		Idempotent:    b.Idempotent,
		Preconditions: b.Preconditions,
		Scopes:        b.Scopes,
	}
}

// resourceOf はパスの第1セグメントを単数形にしてリソース名を返す(例 "/users/{id}" → "user")。
func resourceOf(path string) string {
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || strings.HasPrefix(seg, "{") {
			continue
		}
		return singular(seg)
	}
	return ""
}

// singular は素朴に末尾の "s" を落として単数形にする("users" → "user")。
func singular(s string) string {
	s = strings.ToLower(s)
	if len(s) > 1 && strings.HasSuffix(s, "s") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

// parsePathParams は Path の `{name}` を PathParam(名前のみ)に変換する(Untyped 用)。
func parsePathParams(path string) []PathParam {
	var params []PathParam
	for _, name := range parsePathParamNames(path) {
		params = append(params, PathParam{Name: name})
	}
	return params
}

// parsePathParamNames は Path の `{name}` セグメントの名前を抽出する。
func parsePathParamNames(path string) []string {
	var names []string
	for _, seg := range strings.Split(path, "/") {
		if len(seg) >= 2 && strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			names = append(names, seg[1:len(seg)-1])
		}
	}
	return names
}

// applyFieldDocs は宣言されたフィールド意味(ドット/角括弧記法キー)をスキーマ木に反映する。
func applyFieldDocs(schema *Schema, fieldDocs map[string]string) {
	if schema == nil || len(fieldDocs) == 0 {
		return
	}
	for path, meaning := range fieldDocs {
		if f := findField(schema, path); f != nil {
			f.Meaning = meaning
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
