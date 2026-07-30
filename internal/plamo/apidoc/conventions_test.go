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
		It("builds the error schema tree and attaches an example", func() {
			s := apidoc.CommonErrorSchema(reflect.TypeFor[errEnvelope]())

			Expect(s).NotTo(BeNil())
			Expect(schemaAt(s, "error").Type).To(Equal(apidoc.TypeObject))
			Expect(schemaAt(s, "error.code").Type).To(Equal(apidoc.TypeString))
			Expect(schemaAt(s, "error.message").Type).To(Equal(apidoc.TypeString))
			Expect(s.Example).NotTo(BeNil())
		})
	})
})

var _ = Describe("Behavior wiring", func() {
	idempotent := true

	buildBehaviorModule := func() *rest.Module {
		ep := &restkit.Endpoint[createReq, createRes]{
			Success:  201,
			Security: &restkit.SecuritySpec{Scopes: []string{"users:write"}},
			Behavior: restkit.BehaviorSpec{
				SideEffects:   []string{"sends a confirmation email"},
				Idempotent:    &idempotent,
				Preconditions: []string{"caller must be admin"},
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
		Expect(spec.Security.Scopes).To(ContainElement("users:write"))
	})

	It("leaves Idempotent nil when undeclared (renderer emits TODO)", func() {
		// buildModule (from describe_test) declares no behavior.
		spec := apidoc.DescribeModule(buildModule(), nil)[0]

		Expect(spec.Behavior.Idempotent).To(BeNil())
		Expect(spec.Behavior.SideEffects).To(BeEmpty())
	})
})
