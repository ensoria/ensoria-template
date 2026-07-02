package apidoc_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/validator/pkg/rule"
)

type exUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Age       int       `json:"age"`
	Role      string    `json:"role"`
	Address   exAddress `json:"address"`
	Tags      []string  `json:"tags"`
	CreatedAt string    `json:"created_at"`
}

type exAddress struct {
	City string `json:"city"`
}

var exmsgs = map[string]string{"en": "invalid"}

var _ = Describe("ExampleFromType", func() {
	It("is deterministic for the same type and rules", func() {
		a := apidoc.ExampleFromType(reflect.TypeFor[exUser](), nil, apidoc.ExampleOptions{Resource: "user"})
		b := apidoc.ExampleFromType(reflect.TypeFor[exUser](), nil, apidoc.ExampleOptions{Resource: "user"})

		Expect(a).To(Equal(b))
	})

	Describe("field-name heuristics + fixtures", func() {
		var ex map[string]any

		BeforeEach(func() {
			ex = apidoc.ExampleFromType(reflect.TypeFor[exUser](), nil, apidoc.ExampleOptions{Resource: "user"}).(map[string]any)
		})

		It("prefixes a bare id with the resource name (auto-derived)", func() {
			Expect(ex["id"]).To(Equal("user_01HXYZ7A8B9C0D1E2F3G"))
		})

		It("generates a realistic email", func() {
			Expect(ex["email"]).To(ContainSubstring("@"))
		})

		It("uses the shared timestamp fixture for *_at", func() {
			Expect(ex["created_at"]).To(Equal("2026-06-11T09:30:00Z"))
		})

		It("produces a non-empty name", func() {
			Expect(ex["name"]).NotTo(BeEmpty())
		})

		It("nests objects and emits one-element arrays", func() {
			addr, ok := ex["address"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(addr).To(HaveKey("city"))

			tags, ok := ex["tags"].([]any)
			Expect(ok).To(BeTrue())
			Expect(tags).To(HaveLen(1))
		})
	})

	Describe("constraint satisfaction", func() {
		It("picks an enum value when str_any_of is present", func() {
			rules := []*rule.RuleSet{
				{Field: "role", Rules: []rule.Rule{rule.CreateStrAnyOf(exmsgs)("admin", "member")}},
			}
			ex := apidoc.ExampleFromType(reflect.TypeFor[exUser](), rules, apidoc.ExampleOptions{Resource: "user"}).(map[string]any)

			Expect(ex["role"]).To(BeElementOf("admin", "member"))
		})

		It("respects a max length constraint", func() {
			rules := []*rule.RuleSet{
				{Field: "name", Rules: []rule.Rule{rule.CreateStrMaxLength(exmsgs)(4)}},
			}
			ex := apidoc.ExampleFromType(reflect.TypeFor[exUser](), rules, apidoc.ExampleOptions{Resource: "user"}).(map[string]any)

			Expect(len(ex["name"].(string))).To(BeNumerically("<=", 4))
		})

		It("respects an int range constraint", func() {
			rules := []*rule.RuleSet{
				{Field: "age", Rules: []rule.Rule{rule.CreateIntBetween(exmsgs)(18, 20)}},
			}
			ex := apidoc.ExampleFromType(reflect.TypeFor[exUser](), rules, apidoc.ExampleOptions{Resource: "user"}).(map[string]any)

			Expect(ex["age"]).To(BeNumerically(">=", 18))
			Expect(ex["age"]).To(BeNumerically("<=", 20))
		})
	})

	Describe("id prefixes", func() {
		type withFK struct {
			ID      string `json:"id"`
			OwnerID string `json:"owner_id"`
		}

		It("overrides the bare id prefix via IDPrefixes", func() {
			ex := apidoc.ExampleFromType(reflect.TypeFor[exUser](), nil, apidoc.ExampleOptions{
				Resource:   "user",
				IDPrefixes: map[string]string{"user": "usr"},
			}).(map[string]any)

			Expect(ex["id"]).To(Equal("usr_01HXYZ7A8B9C0D1E2F3G"))
		})

		It("derives a foreign-key prefix from the <name>_id field name", func() {
			ex := apidoc.ExampleFromType(reflect.TypeFor[withFK](), nil, apidoc.ExampleOptions{
				Resource: "order",
			}).(map[string]any)

			Expect(ex["id"]).To(Equal("order_01HXYZ7A8B9C0D1E2F3G"))
			Expect(ex["owner_id"]).To(Equal("owner_01HXYZ7A8B9C0D1E2F3G"))
		})
	})

	It("is wired into DescribeEndpoint request/response schemas", func() {
		specs := apidoc.DescribeModule(buildModule(), nil)
		Expect(specs[0].Request.Example).NotTo(BeNil())
		Expect(specs[0].Response.Example).NotTo(BeNil())
	})
})
