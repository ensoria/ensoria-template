package apidoc_test

import (
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
})
