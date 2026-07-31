package apidoc_test

import (
	"net/http"

	"github.com/ensoria/validator/pkg/optional"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// optionalBody has a pointer field, which is how a client tells "not given"
// apart from a zero value.
type optionalBody struct {
	Count *int    `json:"count"`
	Note  *string `json:"note"`
}

var _ = Describe("nullability and required, taken together", func() {
	describeWith := func(rules []*rule.RuleSet) *apidoc.EndpointSpec {
		ep := &restkit.Endpoint[optionalBody, restkit.NoBody]{
			Success:   http.StatusNoContent,
			Security:  &restkit.SecuritySpec{Public: true},
			BodyRules: rules,
			Handle: func(r *rest.Request, _ *optionalBody) (*rest.Result[restkit.NoBody], error) {
				return restkit.NoContent(), nil
			},
		}
		module := &rest.Module{Path: "/things", Post: restkit.NewController(ep)}
		return apidoc.DescribeModule(module, nil)[0]
	}

	field := func(spec *apidoc.EndpointSpec, name string) *apidoc.Field {
		GinkgoHelper()
		for _, f := range spec.Request.Fields {
			if f.Name == name {
				return f
			}
		}
		Fail("no such field: " + name)
		return nil
	}

	// A pointer field can carry null, and saying so is the honest default.
	It("reports a pointer field as nullable when nothing says otherwise", func() {
		spec := describeWith(nil)

		Expect(field(spec, "count").Schema.Nullable).To(BeTrue())
		Expect(field(spec, "count").Required).To(BeFalse())
	})

	// not_nil rejects null at runtime, so documenting the field as nullable
	// would describe a request the endpoint refuses.
	It("stops calling a field nullable once not_nil is declared", func() {
		spec := describeWith([]*rule.RuleSet{
			{Field: "count", Rules: []rule.Rule{vkit.NotNil()}},
		})

		Expect(field(spec, "count").Required).To(BeTrue())
		Expect(field(spec, "count").Schema.Nullable).To(BeFalse())
	})

	It("leaves the other fields alone", func() {
		spec := describeWith([]*rule.RuleSet{
			{Field: "count", Rules: []rule.Rule{vkit.NotNil()}},
		})

		Expect(field(spec, "note").Schema.Nullable).To(BeTrue())
		Expect(field(spec, "note").Required).To(BeFalse())
	})

	// A constraint says what a value must look like; it does not say the value
	// has to be there.
	It("keeps a constrained but optional field nullable", func() {
		spec := describeWith([]*rule.RuleSet{
			{Field: "count", Rules: []rule.Rule{vkit.MinValue(1)}},
		})

		Expect(field(spec, "count").Required).To(BeFalse())
		Expect(field(spec, "count").Schema.Nullable).To(BeTrue())
	})

	It("marks a field required without a pointer when num_not_zero is used", func() {
		spec := describeWith([]*rule.RuleSet{
			{Field: "count", Rules: []rule.Rule{vkit.NumNotZero()}},
		})

		Expect(field(spec, "count").Required).To(BeTrue())
	})
})

// A partial update needs three states per field, which Optional carries and a
// pointer cannot.
var _ = Describe("Optional fields in the schema", func() {
	type patchBody struct {
		Name     optional.Optional[string] `json:"name"`
		Nickname optional.Optional[string] `json:"nickname"`
	}

	describeWith := func(rules []*rule.RuleSet) *apidoc.EndpointSpec {
		ep := &restkit.Endpoint[patchBody, restkit.NoBody]{
			Success:   http.StatusNoContent,
			Security:  &restkit.SecuritySpec{Public: true},
			BodyRules: rules,
			Handle: func(r *rest.Request, _ *patchBody) (*rest.Result[restkit.NoBody], error) {
				return restkit.NoContent(), nil
			},
		}
		module := &rest.Module{Path: "/things", Patch: restkit.NewController(ep)}
		return apidoc.DescribeModule(module, nil)[0]
	}

	field := func(spec *apidoc.EndpointSpec, name string) *apidoc.Field {
		GinkgoHelper()
		for _, f := range spec.Request.Fields {
			if f.Name == name {
				return f
			}
		}
		Fail("no such field: " + name)
		return nil
	}

	// Optional[string] is a string or null on the wire, never an object with
	// value/set/null in it.
	It("describes the element type, not the wrapper", func() {
		spec := describeWith(nil)

		Expect(field(spec, "name").Schema.Type).To(Equal(apidoc.TypeString))
		Expect(field(spec, "name").Schema.Fields).To(BeEmpty())
	})

	It("treats the field as omittable and nullable by default", func() {
		spec := describeWith(nil)

		Expect(field(spec, "name").Required).To(BeFalse())
		Expect(field(spec, "name").Optional).To(BeTrue())
		Expect(field(spec, "name").Schema.Nullable).To(BeTrue())
	})

	// not_null_if_set allows omitting the field but refuses null, so calling it
	// nullable would describe a request the endpoint rejects.
	It("stops calling a field nullable once it may not be cleared", func() {
		spec := describeWith([]*rule.RuleSet{
			{Field: "name", Rules: []rule.Rule{vkit.NotNullIfSet()}},
		})

		Expect(field(spec, "name").Schema.Nullable).To(BeFalse())
		Expect(field(spec, "name").Required).To(BeFalse())
	})

	It("leaves the other fields alone", func() {
		spec := describeWith([]*rule.RuleSet{
			{Field: "name", Rules: []rule.Rule{vkit.NotNullIfSet()}},
		})

		Expect(field(spec, "nickname").Schema.Nullable).To(BeTrue())
	})

	// The example has to show what a caller sends, not the wrapper's insides.
	It("puts a value of the element type in the example", func() {
		spec := describeWith(nil)

		example, ok := spec.Request.Example.(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(example["name"]).To(BeAssignableToTypeOf(""))
	})
})
