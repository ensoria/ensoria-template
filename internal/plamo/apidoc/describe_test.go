package apidoc_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

type createReq struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type createRes struct {
	ID string `json:"id"`
}

var vmsgs = map[string]string{"en": "invalid"}

// rawController は Documented を実装しない生 Controller(エスケープハッチ)。
type rawController struct{}

func (rawController) Handle(r *rest.Request) *rest.Response { return &rest.Response{Code: 200} }

// buildModule は型付きエンドポイントを1つ持つテスト用モジュールを作る(テスト間で共有)。
func buildModule() *rest.Module {
	ep := &restkit.Endpoint[createReq, createRes]{
		Summary: "Create user",
		Success: 201,
		BodyRules: []*rule.RuleSet{
			{Field: "name", Rules: []rule.Rule{
				rule.CreateStrNotEmpty(vmsgs)(),
				rule.CreateStrMaxLength(vmsgs)(10),
			}},
			{Field: "role", Rules: []rule.Rule{
				rule.CreateStrAnyOf(vmsgs)("admin", "member"),
			}},
		},
		FieldDocs:       map[string]string{"name": "User display name"},
		ResponseHeaders: []restkit.HeaderSpec{{Name: "Location", Meaning: "URL of created user"}},
		Handle: func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
			return rest.NewResult(&createRes{ID: "usr_01"}), nil
		},
	}
	return &rest.Module{Path: "/users/{id}", Post: restkit.NewController(ep)}
}

var _ = Describe("DescribeModule / DescribeEndpoint", func() {
	Describe("a typed endpoint", func() {
		var spec *apidoc.EndpointSpec

		BeforeEach(func() {
			specs := apidoc.DescribeModule(buildModule(), nil)
			Expect(specs).To(HaveLen(1))
			spec = specs[0]
		})

		It("captures method, path, summary, and success status", func() {
			Expect(spec.Method).To(Equal("POST"))
			Expect(spec.Path).To(Equal("/users/{id}"))
			Expect(spec.Summary).To(Equal("Create user"))
			Expect(spec.SuccessStatus).To(Equal(201))
			Expect(spec.Untyped).To(BeFalse())
		})

		It("extracts path parameters from the path", func() {
			Expect(spec.PathParams).To(HaveLen(1))
			Expect(spec.PathParams[0].Name).To(Equal("id"))
		})

		It("reflects PathRules constraints onto path parameters", func() {
			ep := &restkit.Endpoint[createReq, createRes]{
				Success: 200,
				PathRules: []*rule.RuleSet{
					{Field: "id", Rules: []rule.Rule{
						rule.CreateStrNotEmpty(vmsgs)(),
						rule.CreateStrMaxLength(vmsgs)(10),
					}},
				},
				Handle: func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
					return rest.NewResult(&createRes{}), nil
				},
			}
			m := &rest.Module{Path: "/users/{id}", Get: restkit.NewController(ep)}

			s := apidoc.DescribeModule(m, nil)[0]

			Expect(s.PathParams).To(HaveLen(1))
			Expect(s.PathParams[0].Name).To(Equal("id"))
			Expect(s.PathParams[0].Required).To(BeTrue())
			c, ok := constraintByCode2(s.PathParams[0].Constraints, "str_max_length")
			Expect(ok).To(BeTrue())
			Expect(c.Params).To(HaveKeyWithValue("max", 10))
		})

		It("builds query parameters from QueryRules", func() {
			ep := &restkit.Endpoint[createReq, createRes]{
				Success: 200,
				QueryRules: []*rule.RuleSet{
					{Field: "limit", Rules: []rule.Rule{rule.CreateIntMax(vmsgs)(100)}},
				},
				Handle: func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
					return rest.NewResult(&createRes{}), nil
				},
			}
			m := &rest.Module{Path: "/users", Get: restkit.NewController(ep)}

			s := apidoc.DescribeModule(m, nil)[0]

			Expect(s.QueryParams).To(HaveLen(1))
			Expect(s.QueryParams[0].Name).To(Equal("limit"))
			c, ok := constraintByCode2(s.QueryParams[0].Constraints, "int_max")
			Expect(ok).To(BeTrue())
			Expect(c.Params).To(HaveKeyWithValue("limit", 100))
		})

		It("marks required fields and captures length constraints structurally", func() {
			name := fieldByName(spec.Request, "name")
			Expect(name.Required).To(BeTrue())
			c, ok := constraintByCode(name, "str_max_length")
			Expect(ok).To(BeTrue())
			Expect(c.Params).To(HaveKeyWithValue("max", 10))
		})

		It("captures enum rules structurally (values preserved)", func() {
			role := fieldByName(spec.Request, "role")
			c, ok := constraintByCode(role, "str_any_of")
			Expect(ok).To(BeTrue())
			Expect(c.Params).To(HaveKeyWithValue("values", []any{"admin", "member"}))
		})

		It("fills field meaning from FieldDocs", func() {
			Expect(fieldByName(spec.Request, "name").Meaning).To(Equal("User display name"))
		})

		It("builds the response schema and headers", func() {
			Expect(fieldByName(spec.Response, "id").Type).To(Equal("string"))
			Expect(spec.ResponseHeaders).To(HaveLen(1))
			Expect(spec.ResponseHeaders[0].Name).To(Equal("Location"))
		})
	})

	Describe("a raw (non-Documented) controller", func() {
		It("produces an untyped spec with only method and path", func() {
			m := &rest.Module{Path: "/legacy", Get: rawController{}}

			specs := apidoc.DescribeModule(m, nil)

			Expect(specs).To(HaveLen(1))
			Expect(specs[0].Method).To(Equal("GET"))
			Expect(specs[0].Untyped).To(BeTrue())
			Expect(specs[0].Request).To(BeNil())
		})
	})

	Describe("Build over multiple modules", func() {
		It("aggregates endpoints from all modules", func() {
			spec := apidoc.Build([]*rest.Module{buildModule(), {Path: "/legacy", Get: rawController{}}})

			Expect(spec.Endpoints).To(HaveLen(2))
		})
	})

	Describe("Task / Related / Errors declarations", func() {
		var spec *apidoc.EndpointSpec

		BeforeEach(func() {
			ep := &restkit.Endpoint[createReq, createRes]{
				Success:  201,
				Task:     "create user",
				AlsoRead: []string{"workflows/onboarding.md"},
				Related:  []string{"Fetch after creation: GET /users/{id}"},
				Errors: []restkit.ErrorSpec{
					// 共通形に従うエラーは表の1行のみ(Body なし)。
					{Status: 409, Code: "email_taken", Condition: "email exists", CallerAction: "use another email"},
					// field-level エラーは BodyType から example + 表を組む。
					{Status: 422, Code: "validation_failed", BodyType: reflect.TypeFor[envBody]()},
				},
				Handle: func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
					return rest.NewResult(&createRes{}), nil
				},
			}
			m := &rest.Module{Path: "/users", Post: restkit.NewController(ep)}
			spec = apidoc.DescribeModule(m, nil)[0]
		})

		It("carries the Task label, Also read, and Related items through", func() {
			Expect(spec.Task).To(Equal("create user"))
			Expect(spec.AlsoRead).To(Equal([]string{"workflows/onboarding.md"}))
			Expect(spec.Related).To(Equal([]string{"Fetch after creation: GET /users/{id}"}))
		})

		It("maps error rows and leaves the common-shape error without a body", func() {
			Expect(spec.Errors).To(HaveLen(2))
			Expect(spec.Errors[0].Status).To(Equal(409))
			Expect(spec.Errors[0].Code).To(Equal("email_taken"))
			Expect(spec.Errors[0].Body).To(BeNil())
		})

		It("builds an example/field table for an error with a declared body type", func() {
			body := spec.Errors[1].Body
			Expect(body).NotTo(BeNil())
			Expect(body.Fields).NotTo(BeEmpty())
		})

		It("injects the declared code into the error envelope example", func() {
			ex, ok := spec.Errors[1].Body.Example.(map[string]any)
			Expect(ok).To(BeTrue())
			detail, ok := ex["error"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(detail["code"]).To(Equal("validation_failed"))
		})
	})
})

// envBody は docai エラーエンベロープ形のテスト用型(errorExample のコード差し替え確認用)。
type envBody struct {
	Error envDetail `json:"error"`
}

type envDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
