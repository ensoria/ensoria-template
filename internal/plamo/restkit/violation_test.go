package restkit_test

import (
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
)

// asAttrs asserts that every argument is a log attribute and indexes them by
// key, so that a spec can assert on the fields without depending on the order
// they arrive in — or on how slog happens to represent a value internally.
func asAttrs(args []any) map[string]any {
	GinkgoHelper()

	attrs := make(map[string]any, len(args))
	for _, arg := range args {
		attr, ok := arg.(slog.Attr)
		Expect(ok).To(BeTrue(), "expected a log attribute, got %T", arg)
		attrs[attr.Key] = attr.Value.Any()
	}
	return attrs
}

// missingAuthorization stands in for the second kind of contract violation the
// interface has to be able to carry: one that is reported with a 500 and a
// record, and is never panicked.
//
// It is a fixture, not the real check — that one does not exist yet. It is here
// so that the interface is held to the shape such a check needs: fields of its
// own choosing, and no dependence on being panicked to be useful.
type missingAuthorization struct {
	Resource string
}

func (m *missingAuthorization) Error() string {
	return "the declaration requires a resource check that the handler never ran"
}

func (m *missingAuthorization) LogAttrs() []slog.Attr {
	return []slog.Attr{slog.String("resource", m.Resource)}
}

var _ = Describe("ContractViolation", func() {
	// The contract is about naming log fields, not about panicking. A violation
	// that is only ever logged has to fit it too, otherwise the place that
	// expands violations would need a second way to recognise them.
	It("is satisfied by a violation that is reported without ever being panicked", func() {
		var violation restkit.ContractViolation = &missingAuthorization{Resource: "order"}

		Expect(violation.Error()).To(ContainSubstring("never ran"))
		Expect(asAttrs(restkit.LogArgs(violation))).To(Equal(map[string]any{"resource": "order"}))
	})

	Describe("LogArgs", func() {
		It("hands a violation's fields over in the form a logger takes them", func() {
			drift := &restkit.DeclarationDrift{
				Method: http.MethodPost, Path: "/orders", Status: http.StatusAccepted,
			}

			Expect(asAttrs(restkit.LogArgs(drift))).To(Equal(map[string]any{
				"method": http.MethodPost,
				"path":   "/orders",
				"status": int64(http.StatusAccepted),
			}))
		})
	})
})
