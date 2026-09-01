package restkit_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// requestPath is the path the specs' requests are made against, so that a
// record naming the endpoint can be recognised by it.
const requestPath = "/things"

// getRequest builds a bodyless request for the controllers under test.
func getRequest() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, requestPath, nil))
}

// captureLogRecords redirects the global logger into a buffer for the duration
// of write, and returns the records it wrote, decoded.
//
// The logger is global, so the previous one is put back afterwards.
func captureLogRecords(write func()) []map[string]any {
	GinkgoHelper()

	var buf bytes.Buffer
	previous := loggear.GetLogger()
	loggear.SetLogger(loggear.NewSlogLogger(loggear.WithOutput(&buf)))
	defer loggear.SetLogger(previous)

	write()

	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		Expect(json.Unmarshal(line, &record)).To(Succeed())
		records = append(records, record)
	}
	return records
}

// returning builds a controller whose handler answers with the given status.
func returning(status int, responses ...restkit.ResponseSpec) rest.Controller {
	ep := &restkit.Endpoint[restkit.NoBody, okBody]{
		Security:  &restkit.SecuritySpec{Public: true},
		Success:   http.StatusOK,
		Responses: responses,
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[okBody], error) {
			return rest.NewResult(&okBody{OK: true}, rest.WithStatus(status)), nil
		},
	}
	return restkit.NewController(ep)
}

type okBody struct {
	OK bool `json:"ok"`
}

// violationFromPanic runs call, which is expected to panic with a contract
// violation, and returns that violation.
func violationFromPanic(call func()) restkit.ContractViolation {
	GinkgoHelper()

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		call()
	}()

	violation, ok := recovered.(restkit.ContractViolation)
	Expect(ok).To(BeTrue(), "expected a contract violation, got %T: %v", recovered, recovered)
	return violation
}

// attrsOf indexes a violation's fields by key. It goes through LogArgs because
// that is how the fields reach a record in the end; asAttrs lives beside the
// interface it belongs to.
func attrsOf(v restkit.ContractViolation) map[string]any {
	GinkgoHelper()

	return asAttrs(restkit.LogArgs(v))
}

// jsonValue is what a field's value looks like after a round trip through the
// JSON handler, where every number arrives as a float64.
func jsonValue(v any) any {
	if n, ok := v.(int64); ok {
		return float64(n)
	}
	return v
}

// A status the handler returns must be declared, otherwise the generated
// documentation silently drifts from the implementation. The declaration is
// therefore load-bearing: the adapter checks it on every response.
var _ = Describe("undeclared success status", func() {
	// The mode is process-wide, so every spec puts it back where it found it,
	// and every Describe states the mode it needs itself. Leaning on another
	// Describe's cleanup for that would make the outcome depend on the order
	// the specs happen to run in, and --randomize-all would break it.
	var strictBefore bool

	BeforeEach(func() { strictBefore = restkit.StrictDeclarations() })
	AfterEach(func() { restkit.SetStrictDeclarations(strictBefore) })

	Describe("in strict mode (local / test / development)", func() {
		BeforeEach(func() { restkit.SetStrictDeclarations(true) })

		It("panics when the handler returns a status nobody declared", func() {
			ctrl := returning(http.StatusCreated)

			Expect(func() { ctrl.Handle(getRequest()) }).
				To(PanicWith(MatchError(ContainSubstring("undeclared success status 201"))))
		})

		// The panic reaches a log record through middleware that knows nothing
		// about declarations, so the value has to name the endpoint itself. A
		// stack trace cannot stand in for that: the frames of a generic
		// controller say endpointController[...] and never the route.
		It("panics with a violation that names the endpoint and the status", func() {
			ctrl := returning(http.StatusCreated)

			violation := violationFromPanic(func() { ctrl.Handle(getRequest()) })

			Expect(attrsOf(violation)).To(Equal(map[string]any{
				"method": http.MethodGet,
				"path":   requestPath,
				"status": int64(http.StatusCreated),
			}))
		})

		// The record's type belongs to whoever writes the record. A violation
		// that named one too would put the same key in one JSON object twice,
		// and an alert matching on it could not say which occurrence it meant.
		It("panics with a violation that claims no record type of its own", func() {
			ctrl := returning(http.StatusCreated)

			violation := violationFromPanic(func() { ctrl.Handle(getRequest()) })

			Expect(attrsOf(violation)).NotTo(HaveKey("type"))
		})

		// Both branches are built from one value, so the drift is described the
		// same way whether it was panicked or logged.
		It("panics with the same fields the drift record carries", func() {
			strictViolation := violationFromPanic(func() { returning(http.StatusCreated).Handle(getRequest()) })

			restkit.SetStrictDeclarations(false)
			records := captureLogRecords(func() { returning(http.StatusCreated).Handle(getRequest()) })

			Expect(records).To(HaveLen(1))
			for key, value := range attrsOf(strictViolation) {
				Expect(records[0]).To(HaveKeyWithValue(key, jsonValue(value)), "field %s", key)
			}
		})

		It("accepts the declared primary success status", func() {
			ctrl := returning(http.StatusOK)

			Expect(ctrl.Handle(getRequest()).Code).To(Equal(http.StatusOK))
		})

		It("accepts a status declared through Responses", func() {
			ctrl := returning(http.StatusCreated, restkit.ResponseSpec{
				Status: http.StatusCreated, When: "a new user was created",
			})

			Expect(ctrl.Handle(getRequest()).Code).To(Equal(http.StatusCreated))
		})

		It("accepts the implicit 200 when Success is left unset", func() {
			ep := &restkit.Endpoint[restkit.NoBody, okBody]{
				Security: &restkit.SecuritySpec{Public: true},
				Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[okBody], error) {
					return rest.NewResult(&okBody{OK: true}), nil
				},
			}

			Expect(restkit.NewController(ep).Handle(getRequest()).Code).To(Equal(http.StatusOK))
		})
	})

	Describe("StrictForEnv", func() {
		It("is strict only in the environments a developer works in", func() {
			Expect(restkit.StrictForEnv("local")).To(BeTrue())
			Expect(restkit.StrictForEnv("test")).To(BeTrue())
			Expect(restkit.StrictForEnv("development")).To(BeTrue())
			Expect(restkit.StrictForEnv("staging")).To(BeFalse())
			Expect(restkit.StrictForEnv("production")).To(BeFalse())
			Expect(restkit.StrictForEnv("")).To(BeFalse())
		})
	})

	Describe("outside strict mode (staging / production)", func() {
		BeforeEach(func() { restkit.SetStrictDeclarations(false) })

		It("still answers with the undeclared status instead of failing the request", func() {
			ctrl := returning(http.StatusCreated)

			var res *rest.Response
			Expect(func() { res = ctrl.Handle(getRequest()) }).NotTo(Panic())
			Expect(res.Code).To(Equal(http.StatusCreated))
		})

		// Here the record is the only sign that the documentation has drifted,
		// so it has to say which endpoint drifted and be findable by a condition
		// that does not depend on its wording.
		It("names the endpoint the drift came from", func() {
			ctrl := returning(http.StatusCreated)

			records := captureLogRecords(func() { ctrl.Handle(getRequest()) })

			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("method", http.MethodGet))
			Expect(records[0]).To(HaveKeyWithValue("path", requestPath))
			Expect(records[0]).To(HaveKeyWithValue("status", float64(http.StatusCreated)))
			Expect(records[0]).To(HaveKeyWithValue("level", "ERROR"))
		})

		It("carries the stable type the template's other records carry", func() {
			ctrl := returning(http.StatusCreated)

			records := captureLogRecords(func() { ctrl.Handle(getRequest()) })

			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("type", restkit.LogTypeDeclarationDrift))
		})

		It("writes nothing when the status was declared", func() {
			ctrl := returning(http.StatusOK)

			Expect(captureLogRecords(func() { ctrl.Handle(getRequest()) })).To(BeEmpty())
		})
	})
})

// The check above is only worth what it reaches, and what it reaches is decided
// by the default: nothing in a test asks for strict mode, so a suite added
// tomorrow inherits the check only if the package turns it on by itself.
//
// This asserts on the default the package chose at initialisation rather than
// on the current value of the flag, which the specs above overwrite.
var _ = Describe("the default strict declaration mode", func() {
	It("is on inside a test binary, without any suite asking for it", func() {
		Expect(restkit.InitialStrictDeclarations()).To(BeTrue())
	})
})
