package apidoc_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// errDetail / errEnvelope は共通エラー本文形の例。
type errDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errEnvelope struct {
	Error errDetail `json:"error"`
}

var _ = Describe("Conventions", func() {
	Describe("CommonErrorSchema", func() {
		It("flattens the error type and attaches an example", func() {
			s := apidoc.CommonErrorSchema(reflect.TypeFor[errEnvelope]())

			Expect(s).NotTo(BeNil())
			Expect(fieldByName(s, "error").Type).To(Equal("object"))
			Expect(fieldByName(s, "error.code").Type).To(Equal("string"))
			Expect(fieldByName(s, "error.message").Type).To(Equal("string"))
			Expect(s.Example).NotTo(BeNil())
		})
	})
})

var _ = Describe("Behavior wiring", func() {
	idempotent := true

	buildBehaviorModule := func() *rest.Module {
		ep := &restkit.Endpoint[createReq, createRes]{
			Success: 201,
			Behavior: restkit.BehaviorSpec{
				SideEffects:   []string{"sends a confirmation email"},
				Idempotent:    &idempotent,
				Preconditions: []string{"caller must be admin"},
				Scopes:        []string{"users:write"},
			},
			Handle: func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
				return rest.NewResult(&createRes{ID: "usr_01"}), nil
			},
		}
		return &rest.Module{Path: "/users", Post: restkit.NewController(ep)}
	}

	It("carries declared behavior onto the endpoint spec", func() {
		spec := apidoc.DescribeModule(buildBehaviorModule(), nil)[0]

		Expect(spec.Behavior.SideEffects).To(ContainElement("sends a confirmation email"))
		Expect(spec.Behavior.Idempotent).NotTo(BeNil())
		Expect(*spec.Behavior.Idempotent).To(BeTrue())
		Expect(spec.Behavior.Preconditions).To(ContainElement("caller must be admin"))
		Expect(spec.Behavior.Scopes).To(ContainElement("users:write"))
	})

	It("leaves Idempotent nil when undeclared (renderer emits TODO)", func() {
		// buildModule (from describe_test) declares no behavior.
		spec := apidoc.DescribeModule(buildModule(), nil)[0]

		Expect(spec.Behavior.Idempotent).To(BeNil())
		Expect(spec.Behavior.SideEffects).To(BeEmpty())
	})
})
