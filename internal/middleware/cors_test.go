package middleware_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/rest/pkg/rest"
)

// corsConfig is a deployment that serves its frontend from another origin.
func corsConfig() *appconfig.CORS {
	return &appconfig.CORS{
		AllowOriginVal:      "https://app.example.test,https://admin.example.test",
		AllowMethodsVal:     "GET,POST,DELETE,OPTIONS",
		AllowHeadersVal:     "Content-Type,Authorization",
		ExposeHeadersVal:    "X-Request-Id",
		MaxAgeVal:           600,
		AllowCredentialsVal: true,
	}
}

// corsRequest builds a request said to come from origin ("" sends no header).
func corsRequest(method, origin string, headers map[string]string) *rest.Request {
	req := httptest.NewRequest(method, siteHost+"/things", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return rest.NewRequest(req)
}

// handled answers 200 and records that the handler ran.
func handled(reached *bool) rest.Handler {
	return func(*rest.Request) *rest.Response {
		*reached = true
		return &rest.Response{Code: http.StatusOK}
	}
}

// preflight is the header that makes an OPTIONS request a preflight.
var preflight = map[string]string{"Access-Control-Request-Method": "POST"}

var _ = Describe("CORS", func() {
	run := func(cfg *appconfig.CORS, r *rest.Request, reached *bool) *rest.Response {
		return middleware.CORS(cfg)(handled(reached))(r)
	}

	// ⚠ The defect this pins, and the reason the whole thing was rewritten. A
	// browser needs the header on the response it is going to read, not only on
	// the preflight — and a server-side test that checks only the preflight
	// sees a configuration that looks entirely correct.
	Describe("an actual request", func() {
		It("carries the headers a browser needs to read the response", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodGet, "https://app.example.test", nil), &reached)

			Expect(reached).To(BeTrue())
			Expect(res.AddHeaders).To(HaveKeyWithValue(
				"Access-Control-Allow-Origin", "https://app.example.test"))
			Expect(res.AddHeaders).To(HaveKeyWithValue("Access-Control-Allow-Credentials", "true"))
			Expect(res.AddHeaders).To(HaveKeyWithValue("Access-Control-Expose-Headers", "X-Request-Id"))
		})

		// They answer a preflight's question about what would be permitted, and
		// a browser ignores them anywhere else.
		It("leaves the preflight-only headers off", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodGet, "https://app.example.test", nil), &reached)

			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Methods"))
			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Headers"))
			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Max-Age"))
		})

		// ⚠ CORS refuses nothing: it is enforced by the browser, and a caller
		// that is not one ignores the headers entirely. What refuses is the
		// cross-origin check, and only for requests that change state.
		It("serves an origin nobody claimed, and tells the browser nothing", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodGet, "https://evil.example", nil), &reached)

			Expect(reached).To(BeTrue())
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Origin"))
		})

		// Otherwise a cache could replay the header-less answer given to an
		// unknown origin to the frontend, and the frontend would break for
		// exactly as long as that entry lived.
		It("says the answer varies by origin, even when this one was refused", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodGet, "https://evil.example", nil), &reached)

			Expect(res.AddHeaders).To(HaveKeyWithValue("Vary", "Origin"))
		})

		It("adds nothing to a request that reports no origin", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodGet, "", nil), &reached)

			Expect(reached).To(BeTrue())
			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Origin"))
		})
	})

	Describe("a preflight", func() {
		It("is answered without reaching the handler", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodOptions, "https://app.example.test", preflight), &reached)

			Expect(reached).To(BeFalse())
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(res.AddHeaders).To(HaveKeyWithValue("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS"))
			Expect(res.AddHeaders).To(HaveKeyWithValue("Access-Control-Allow-Headers", "Content-Type,Authorization"))
			Expect(res.AddHeaders).To(HaveKeyWithValue("Access-Control-Max-Age", "600"))
		})

		// An application is allowed to serve OPTIONS itself; a preflight is the
		// one that names the method it is asking about.
		It("does not swallow an OPTIONS request that is not one", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodOptions, "https://app.example.test", nil), &reached)

			Expect(reached).To(BeTrue())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("answers an unclaimed origin with no permission at all", func() {
			reached := false

			res := run(corsConfig(), corsRequest(http.MethodOptions, "https://evil.example", preflight), &reached)

			Expect(reached).To(BeFalse())
			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Origin"))
			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Methods"))
		})
	})

	Describe("the wildcard", func() {
		wildcard := func() *appconfig.CORS {
			cfg := corsConfig()
			cfg.AllowOriginVal = "*"
			return cfg
		}

		It("answers every origin with the wildcard itself", func() {
			reached := false

			res := run(wildcard(), corsRequest(http.MethodGet, "https://anywhere.example", nil), &reached)

			Expect(res.AddHeaders).To(HaveKeyWithValue("Access-Control-Allow-Origin", "*"))
			Expect(res.AddHeaders).NotTo(HaveKey("Vary"))
		})

		// ⚠ A browser refuses the combination outright, so offering it would
		// break the very requests it was meant to enable.
		It("never offers credentials alongside it", func() {
			reached := false

			res := run(wildcard(), corsRequest(http.MethodGet, "https://anywhere.example", nil), &reached)

			Expect(res.AddHeaders).NotTo(HaveKey("Access-Control-Allow-Credentials"))
		})
	})

	// The same-origin deployment. Nothing cross-origin is meant to work, and
	// the middleware has nothing to say about any request.
	Describe("a deployment that claims no origin", func() {
		It("leaves every response untouched", func() {
			reached := false

			res := run(&appconfig.CORS{}, corsRequest(http.MethodGet, "https://app.example.test", nil), &reached)

			Expect(reached).To(BeTrue())
			Expect(res.AddHeaders).To(BeEmpty())
		})

		It("does not answer preflights either", func() {
			reached := false

			run(&appconfig.CORS{}, corsRequest(http.MethodOptions, "https://app.example.test", preflight), &reached)

			Expect(reached).To(BeTrue())
		})
	})

	// ⚠ A handler that set ReplaceHeaders asked for the base headers to be
	// cleared, and the pipeline then ignores AddHeaders entirely — so headers
	// written there would vanish for exactly the endpoints that customise them.
	Describe("a handler that replaces its headers", func() {
		It("puts the CORS headers where that response will actually read them", func() {
			replacing := func(*rest.Request) *rest.Response {
				return &rest.Response{Code: http.StatusOK, ReplaceHeaders: map[string]string{"X-Own": "1"}}
			}

			res := middleware.CORS(corsConfig())(replacing)(
				corsRequest(http.MethodGet, "https://app.example.test", nil))

			Expect(res.ReplaceHeaders).To(HaveKeyWithValue(
				"Access-Control-Allow-Origin", "https://app.example.test"))
			Expect(res.ReplaceHeaders).To(HaveKeyWithValue("X-Own", "1"))
		})
	})
})
