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

var _ = Describe("ContractViolation", func() {
	// The contract is about naming log fields, not about panicking. A violation
	// that is only ever logged has to fit it too, otherwise the place that
	// expands violations would need a second way to recognise them.
	//
	// This was held by a fixture until the resource check existed; it is now
	// held by the real thing, which is reported with a 500 and a record and is
	// never panicked.
	It("is satisfied by a violation that is reported without ever being panicked", func() {
		var violation restkit.ContractViolation = &restkit.MissingAuthorization{
			Method: http.MethodPut, Path: "/order/{id}",
		}

		Expect(violation.Error()).To(ContainSubstring("never ran"))
		Expect(asAttrs(restkit.LogArgs(violation))).To(Equal(map[string]any{
			"method": http.MethodPut,
			"path":   "/order/{id}",
		}))
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
