package apidoc_test

import (
	"net/http"

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
