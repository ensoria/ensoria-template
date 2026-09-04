package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// The host the requests below are addressed to. Whether a request is
// cross-origin is decided by comparing what the browser reported against this.
const siteHost = "https://api.example.test"

// crossOriginRequest builds a request said to come from origin. An empty origin
// sends no header at all, which is what a caller that is not a browser does.
func crossOriginRequest(method, origin string) *rest.Request {
	req := httptest.NewRequest(method, siteHost+"/things", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return rest.NewRequest(req)
}

// served answers 200 and records that it ran (test helper).
func served(reached *bool) rest.Handler {
	return func(*rest.Request) *rest.Response {
		*reached = true
		return &rest.Response{Code: http.StatusOK}
	}
}

// refusingChecker refuses every request it is shown (test helper).
type refusingChecker struct{}

func (refusingChecker) Check(*http.Request) error { return errors.New("cross-origin request") }

var _ = Describe("NewCrossOriginProtection", func() {
	It("trusts each origin the CORS setting names", func() {
		protection, err := middleware.NewCrossOriginProtection(
			"https://app.example.test, https://admin.example.test")
		Expect(err).NotTo(HaveOccurred())

		for _, origin := range []string{"https://app.example.test", "https://admin.example.test"} {
			req := httptest.NewRequest(http.MethodPost, siteHost+"/things", nil)
			req.Header.Set("Origin", origin)
			Expect(protection.Check(req)).To(Succeed(), origin)
		}
	})

	// The same-origin deployment writes nothing here, and a request from the
	// origin serving the page is not cross-origin in the first place.
	It("accepts an empty setting", func() {
		protection, err := middleware.NewCrossOriginProtection("")

		Expect(err).NotTo(HaveOccurred())
		Expect(protection).NotTo(BeNil())
	})

	// ⚠ A wildcard cannot be a trusted origin: "every site is this
	// application's frontend" is not an answer to the question this asks. A
	// deployment that combines it with cookie authentication is refused at
	// startup, so reaching here means sessions are off.
	It("skips the wildcard rather than failing over it", func() {
		protection, err := middleware.NewCrossOriginProtection("*")
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, siteHost+"/things", nil)
		req.Header.Set("Origin", "https://evil.example")
		Expect(protection.Check(req)).To(HaveOccurred())
	})

	It("reports a value that is not an origin, naming the key it came from", func() {
		_, err := middleware.NewCrossOriginProtection("app.example.test/path")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("CORS_ALLOW_ORIGIN"))
	})
})

var _ = Describe("CSRF", func() {
	protection := func() middleware.CrossOriginChecker {
		GinkgoHelper()
		p, err := middleware.NewCrossOriginProtection("https://app.example.test")
		Expect(err).NotTo(HaveOccurred())
		return p
	}

	It("refuses a state-changing request from an origin nobody trusts", func() {
		reached := false

		res := middleware.CSRF(protection())(served(&reached))(
			crossOriginRequest(http.MethodPost, "https://evil.example"))

		Expect(reached).To(BeFalse())
		Expect(res.Code).To(Equal(http.StatusForbidden))
		envelope, ok := res.Body.(*restkit.ErrorEnvelope)
		Expect(ok).To(BeTrue(), "the refusal has to use the shared error shape")
		Expect(envelope.Error.Code).To(Equal(restkit.CrossOriginCode))
	})

	It("serves one from an origin the configuration names", func() {
		reached := false

		res := middleware.CSRF(protection())(served(&reached))(
			crossOriginRequest(http.MethodPost, "https://app.example.test"))

		Expect(reached).To(BeTrue())
		Expect(res.Code).To(Equal(http.StatusOK))
	})

	// Servers, CLIs and mobile clients report no origin at all. Refusing them
	// would break every caller that is not a browser, and none of them can be
	// made to send a request by a page the user visited.
	It("serves a caller that reports no origin", func() {
		reached := false

		res := middleware.CSRF(protection())(served(&reached))(
			crossOriginRequest(http.MethodPost, ""))

		Expect(reached).To(BeTrue())
		Expect(res.Code).To(Equal(http.StatusOK))
	})

	// ⚠ Documented rather than desired: the check is built on safe methods
	// changing nothing. It is why a WebSocket upgrade, which is a GET, needs an
	// origin check of its own.
	It("lets a cross-origin GET through", func() {
		reached := false

		res := middleware.CSRF(protection())(served(&reached))(
			crossOriginRequest(http.MethodGet, "https://evil.example"))

		Expect(reached).To(BeTrue())
		Expect(res.Code).To(Equal(http.StatusOK))
	})

	It("refuses whatever the checker refuses, whoever the checker is", func() {
		reached := false

		res := middleware.CSRF(refusingChecker{})(served(&reached))(
			crossOriginRequest(http.MethodPost, ""))

		Expect(reached).To(BeFalse())
		Expect(res.Code).To(Equal(http.StatusForbidden))
	})

	// A project that replaced the check with nothing gets no check, rather than
	// a panic on the first request.
	It("serves everything when there is no checker", func() {
		reached := false

		res := middleware.CSRF(nil)(served(&reached))(
			crossOriginRequest(http.MethodPost, "https://evil.example"))

		Expect(reached).To(BeTrue())
		Expect(res.Code).To(Equal(http.StatusOK))
	})
})
