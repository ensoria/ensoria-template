package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/pipeline"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/golang-jwt/jwt/v5"
)

// The tests below drive the whole stack over real HTTP: an http.Server built
// from the same pipeline the application uses, a real verifier reading a real
// Authorization header, and endpoints declared the way a project declares them.
//
// The parts each have their own tests; what this file checks is that they add
// up to the answers the documentation promises.

const (
	e2eSecret = "an-end-to-end-test-secret"
	e2eIssuer = "https://issuer.example.test"
)

// e2eBody is the body the writable endpoint takes.
type e2eBody struct {
	Name string `json:"name"`
}

// e2eAuth is the configuration the server under test verifies against.
func e2eAuth() *appconfig.Auth {
	return &appconfig.Auth{
		Mode:         appconfig.AuthModeHS256,
		Secret:       e2eSecret,
		Issuer:       e2eIssuer,
		APIKeyHeader: appconfig.DefaultAPIKeyHeader,
		APIKeys:      []string{"an-end-to-end-test-key"},
	}
}

// token mints a JWT the server will accept, carrying the given scopes.
func token(scopes string) string {
	GinkgoHelper()
	claims := jwt.MapClaims{
		"sub": "usr_1",
		"iss": e2eIssuer,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if scopes != "" {
		claims["scope"] = scopes
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(e2eSecret))
	Expect(err).NotTo(HaveOccurred())
	return signed
}

// serve starts an HTTP server carrying the endpoints under test.
func serve() *httptest.Server {
	GinkgoHelper()

	verifier, err := authkit.NewVerifier(e2eAuth(), nil, nil)
	Expect(err).NotTo(HaveOccurred())

	ok := func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[e2eBody], error) {
		return rest.NewResult(&e2eBody{Name: "hoge"}), nil
	}
	endpoint := func(security *restkit.SecuritySpec) rest.Controller {
		return restkit.NewController(&restkit.Endpoint[restkit.NoBody, e2eBody]{
			Success: http.StatusOK, Security: security, Handle: ok,
		})
	}

	modules := []*rest.Module{
		{Path: "/public", Get: endpoint(&restkit.SecuritySpec{Public: true})},
		{Path: "/private", Get: endpoint(nil)},
		{Path: "/scoped", Get: endpoint(&restkit.SecuritySpec{Scopes: []string{"things:write"}})},
		{Path: "/api-key-only", Get: endpoint(&restkit.SecuritySpec{
			Schemes: []string{authkit.SchemeAPIKey},
		})},
		{Path: "/validated", Post: restkit.NewController(&restkit.Endpoint[e2eBody, e2eBody]{
			Success:   http.StatusOK,
			Security:  &restkit.SecuritySpec{Scopes: []string{"things:write"}},
			BodyRules: []*rule.RuleSet{{Field: "name", Rules: []rule.Rule{vkit.Required()}}},
			Handle: func(r *rest.Request, body *e2eBody) (*rest.Result[e2eBody], error) {
				return rest.NewResult(body), nil
			},
		})},
	}

	httpPipeline := &pipeline.HTTP{
		Modules: modules,
		GlobalMiddlewares: globalMiddlewares(
			&appconfig.CORS{AllowOriginVal: "*"},
			http.NewCrossOriginProtection(),
			verifier,
			&rest.Response{Code: http.StatusInternalServerError},
		),
	}
	mux := http.NewServeMux()
	httpPipeline.Register(mux)
	return httptest.NewServer(mux)
}

// call sends a request and returns its status and body.
func call(server *httptest.Server, method, path string, headers map[string]string, body string) (int, string) {
	GinkgoHelper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := server.Client().Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = res.Body.Close() }()

	payload, err := io.ReadAll(res.Body)
	Expect(err).NotTo(HaveOccurred())
	return res.StatusCode, string(payload)
}

// bearer and apiKey build the headers a caller sends.
func bearer(scopes string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token(scopes)}
}

func apiKey(key string) map[string]string {
	return map[string]string{appconfig.DefaultAPIKeyHeader: key}
}

// errorCode reads the code out of the shared error envelope.
func errorCode(body string) string {
	GinkgoHelper()
	var envelope restkit.ErrorEnvelope
	Expect(json.Unmarshal([]byte(body), &envelope)).To(Succeed())
	return envelope.Error.Code
}

var _ = Describe("authentication and authorization over HTTP", func() {
	var server *httptest.Server

	BeforeEach(func() { server = serve() })
	AfterEach(func() { server.Close() })

	Describe("a public endpoint", func() {
		It("is served without any credential", func() {
			status, _ := call(server, http.MethodGet, "/public", nil, "")

			Expect(status).To(Equal(http.StatusOK))
		})

		// A credential that cannot be trusted is a caller bug. Ignoring it on a
		// public endpoint would hide that bug.
		It("still refuses a credential that cannot be verified", func() {
			status, body := call(server, http.MethodGet, "/public",
				map[string]string{"Authorization": "Bearer not-a-token"}, "")

			Expect(status).To(Equal(http.StatusUnauthorized))
			Expect(errorCode(body)).To(Equal(restkit.UnauthenticatedCode))
		})
	})

	Describe("an endpoint that needs a caller", func() {
		It("answers 401 to an anonymous request", func() {
			status, body := call(server, http.MethodGet, "/private", nil, "")

			Expect(status).To(Equal(http.StatusUnauthorized))
			Expect(errorCode(body)).To(Equal(restkit.UnauthenticatedCode))
		})

		It("answers 200 to a request carrying a valid token", func() {
			status, _ := call(server, http.MethodGet, "/private", bearer(""), "")

			Expect(status).To(Equal(http.StatusOK))
		})

		It("sends WWW-Authenticate so the caller knows what to present", func() {
			res, err := server.Client().Get(server.URL + "/private")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = res.Body.Close() }()

			Expect(res.Header.Get("WWW-Authenticate")).To(ContainSubstring("Bearer"))
		})
	})

	Describe("an endpoint that requires a scope", func() {
		It("answers 401 when nobody is authenticated", func() {
			status, _ := call(server, http.MethodGet, "/scoped", nil, "")

			Expect(status).To(Equal(http.StatusUnauthorized))
		})

		// 403, not 401: the caller is known, and presenting the credential
		// again would not change the answer.
		It("answers 403 to a caller without the scope", func() {
			status, body := call(server, http.MethodGet, "/scoped", bearer("things:read"), "")

			Expect(status).To(Equal(http.StatusForbidden))
			Expect(errorCode(body)).To(Equal(restkit.ForbiddenCode))
		})

		It("answers 200 to a caller holding the scope", func() {
			status, _ := call(server, http.MethodGet, "/scoped", bearer("things:read things:write"), "")

			Expect(status).To(Equal(http.StatusOK))
		})
	})

	Describe("an endpoint that accepts only one kind of credential", func() {
		It("answers 200 to the accepted kind", func() {
			status, _ := call(server, http.MethodGet, "/api-key-only",
				apiKey("an-end-to-end-test-key"), "")

			Expect(status).To(Equal(http.StatusOK))
		})

		// The token is valid; it is the wrong kind of credential for this
		// endpoint, which is an authorization failure rather than an
		// authentication one.
		It("answers 403 to a valid token of another kind", func() {
			status, body := call(server, http.MethodGet, "/api-key-only", bearer(""), "")

			Expect(status).To(Equal(http.StatusForbidden))
			Expect(errorCode(body)).To(Equal(restkit.ForbiddenCode))
		})

		It("answers 401 to an unknown key", func() {
			status, _ := call(server, http.MethodGet, "/api-key-only", apiKey("not-a-key"), "")

			Expect(status).To(Equal(http.StatusUnauthorized))
		})
	})

	// Validation runs after authorization, so an anonymous caller learns
	// nothing about the fields or their constraints.
	Describe("the order of authorization and validation", func() {
		It("answers 401 rather than 422 to an anonymous caller sending an invalid body", func() {
			status, body := call(server, http.MethodPost, "/validated", nil, `{}`)

			Expect(status).To(Equal(http.StatusUnauthorized))
			Expect(body).NotTo(ContainSubstring("name"))
		})

		It("answers 422 once the caller is allowed through", func() {
			status, body := call(server, http.MethodPost, "/validated",
				bearer("things:write"), `{}`)

			Expect(status).To(Equal(http.StatusUnprocessableEntity))
			Expect(body).To(ContainSubstring("name"))
		})

		It("answers 200 to an authorized caller sending a valid body", func() {
			status, _ := call(server, http.MethodPost, "/validated",
				bearer("things:write"), `{"name":"taro"}`)

			Expect(status).To(Equal(http.StatusOK))
		})
	})

	// An expired token is not a token the server may trust, whatever it says.
	It("refuses an expired token", func() {
		claims := jwt.MapClaims{
			"sub": "usr_1",
			"iss": e2eIssuer,
			"exp": time.Now().Add(-time.Hour).Unix(),
		}
		expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(e2eSecret))
		Expect(err).NotTo(HaveOccurred())

		status, _ := call(server, http.MethodGet, "/private",
			map[string]string{"Authorization": "Bearer " + expired}, "")

		Expect(status).To(Equal(http.StatusUnauthorized))
	})
})
