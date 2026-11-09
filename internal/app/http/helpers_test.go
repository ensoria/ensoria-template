package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// rejectingVerifier refuses every credential it is shown.
type rejectingVerifier struct{}

func (rejectingVerifier) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return nil, errors.New("credential could not be verified")
}

func (rejectingVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

func (rejectingVerifier) Recheck(context.Context, *authkit.Credential) (*authkit.Principal, error) {
	return nil, authkit.ErrInvalidCredential
}

// anonymousVerifier reports that the request carried no credential.
type anonymousVerifier struct{}

func (anonymousVerifier) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return &authkit.VerifyResult{}, nil
}

func (anonymousVerifier) Schemes() []string { return []string{authkit.SchemeJWT} }

func (anonymousVerifier) Recheck(context.Context, *authkit.Credential) (*authkit.Principal, error) {
	return nil, authkit.ErrInvalidCredential
}

// chain composes the middlewares the way the pipeline does: the list runs
// outside-in, so it is applied in reverse.
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

// captureLogLines redirects the global logger into a buffer for the duration of
// write and returns the raw lines it wrote.
//
// The lines are returned undecoded because one of the specs is about a key
// appearing exactly once: decoding into a map would collapse a duplicate key
// and hide the very thing being asserted on.
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
