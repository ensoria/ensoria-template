package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// rejectingVerifier refuses every credential it is shown (test helper).
type rejectingVerifier struct{}

func (rejectingVerifier) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return nil, errors.New("credential could not be verified")
}

func (rejectingVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

// anonymousVerifier reports that the request carried no credential (test helper).
type anonymousVerifier struct{}

func (anonymousVerifier) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return &authkit.VerifyResult{}, nil
}

func (anonymousVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

// chain composes the middlewares the way the pipeline does: the list runs
// outside-in, so it is applied in reverse (test helper).
func chain(middlewares []rest.Middleware, final rest.Handler) rest.Handler {
	next := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		next = middlewares[i](next)
	}
	return next
}

func request() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, "/things", nil))
}

var _ = Describe("globalMiddlewares", func() {
	settings := &appconfig.CORS{AllowOriginVal: "*"}
	panicResponse := &rest.Response{Code: http.StatusInternalServerError}

	// Accepting the verifier and then forgetting to install the middleware is a
	// silent hole: the application compiles and serves every request unchecked.
	It("refuses a request whose credential cannot be trusted", func() {
		reached := false

		res := chain(globalMiddlewares(settings, http.NewCrossOriginProtection(), rejectingVerifier{}, panicResponse),
			func(*rest.Request) *rest.Response {
				reached = true
				return &rest.Response{Code: http.StatusOK}
			})(request())

		Expect(res.Code).To(Equal(http.StatusUnauthorized))
		Expect(reached).To(BeFalse(), "the request reached the handler without being authenticated")
	})

	It("keeps serving a request that presents no credential", func() {
		res := chain(globalMiddlewares(settings, http.NewCrossOriginProtection(), anonymousVerifier{}, panicResponse),
			func(*rest.Request) *rest.Response { return &rest.Response{Code: http.StatusOK} })(request())

		Expect(res.Code).To(Equal(http.StatusOK),
			"a public endpoint must still be reachable without a credential")
	})
})

// captureLogLines redirects the global logger into a buffer for the duration of
// write and returns the raw lines it wrote.
//
// The lines are returned undecoded because one of the specs below is about a
// key appearing exactly once: decoding into a map would collapse a duplicate
// key and hide the very thing being asserted on.
func captureLogLines(write func()) [][]byte {
	GinkgoHelper()

	var buf bytes.Buffer
	previous := loggear.GetLogger()
	loggear.SetLogger(loggear.NewSlogLogger(loggear.WithOutput(&buf)))
	defer loggear.SetLogger(previous)

	write()

	var lines [][]byte
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

// decodeRecord decodes one captured line.
func decodeRecord(line []byte) map[string]any {
	GinkgoHelper()

	var record map[string]any
	Expect(json.Unmarshal(line, &record)).To(Succeed())
	return record
}

// A panic reaches this function through middleware that knows nothing about
// declarations or authorization, so whatever the panic value can say about
// itself is all the record can say. Expanding it here is what turns a stack
// trace of generic controller frames into a record naming the endpoint.
var _ = Describe("logPanicDetails", func() {
	drift := &restkit.DeclarationDrift{
		Method: http.MethodPost, Path: "/things", Status: http.StatusAccepted,
	}

	It("expands the fields of a contract violation the panic carried", func() {
		lines := captureLogLines(func() { logPanicDetails(request(), drift, []byte("stack")) })

		Expect(lines).To(HaveLen(1))
		Expect(decodeRecord(lines[0])).To(HaveKeyWithValue(panicViolationGroup, map[string]any{
			"method": http.MethodPost,
			"path":   "/things",
			"status": float64(http.StatusAccepted),
		}))
	})

	// An alert on panics matches on the record's type, so the record must carry
	// exactly one. The count is taken on the raw line: the violation must
	// contribute no type at any level, and decoding first would collapse a
	// duplicate key and hide it.
	It("writes one type, and it is the one the caller chose", func() {
		lines := captureLogLines(func() { logPanicDetails(request(), drift, []byte("stack")) })

		Expect(bytes.Count(lines[0], []byte(`"type":`))).To(Equal(1))
		Expect(decodeRecord(lines[0])).To(HaveKeyWithValue("type", "panic_log"))
	})

	// The violation names an endpoint too, and it need not be the one the
	// request was made against — a group keeps the two apart instead of leaving
	// one "method" to overwrite the other.
	It("leaves the record's own fields describing the request", func() {
		lines := captureLogLines(func() { logPanicDetails(request(), drift, []byte("stack")) })

		record := decodeRecord(lines[0])
		Expect(record).To(HaveKeyWithValue("method", http.MethodGet))
		Expect(record[panicViolationGroup]).To(HaveKeyWithValue("method", http.MethodPost))
	})

	It("keeps the record it always wrote for a panic that is not a violation", func() {
		lines := captureLogLines(func() { logPanicDetails(request(), "boom", []byte("stack")) })

		record := decodeRecord(lines[0])
		Expect(record).To(HaveKeyWithValue("type", "panic_log"))
		Expect(record).To(HaveKeyWithValue("panic_value", "boom"))
		Expect(record).To(HaveKeyWithValue("stack_trace", "stack"))
		Expect(record).NotTo(HaveKey(panicViolationGroup))
	})
})
