package middleware_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// verifierStub answers with whatever the test wants, so the middleware can be
// exercised without minting real tokens.
type verifierStub struct {
	principal *authkit.Principal
	discard   []*http.Cookie
	err       error
}

func (v verifierStub) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	if v.err != nil {
		return nil, v.err
	}
	return &authkit.VerifyResult{Principal: v.principal, Discard: v.discard}, nil
}

func (verifierStub) Schemes() []string { return []string{authkit.SchemeJWT} }

// authRequest builds a request the auth middleware can run against (test helper).
func authRequest() *rest.Request {
	return rest.NewRequest(httptest.NewRequest(http.MethodGet, "/things", nil))
}

// okHandler records the principal it saw and answers 200 (test helper).
func okHandler(seen *authkit.Principal) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		if p, ok := authkit.PrincipalFrom(r.Context()); ok {
			*seen = *p
		}
		return &rest.Response{Code: http.StatusOK}
	}
}

var _ = Describe("authentication middleware", func() {
	Describe("in the HTTP pipeline", func() {
		It("records the verified caller on the request context", func() {
			var seen authkit.Principal
			mw := middleware.Auth(verifierStub{
				principal: &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT},
			})

			res := mw(okHandler(&seen))(authRequest())

			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(seen.Subject).To(Equal("usr_1"))
		})

		// A request with no credential is not refused here: a public endpoint is
		// served without one, so the endpoint decides (see Endpoint.Security).
		It("lets a request with no credential through, carrying no caller", func() {
			var seen authkit.Principal
			mw := middleware.Auth(verifierStub{})

			res := mw(okHandler(&seen))(authRequest())

			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(seen.Subject).To(BeEmpty())
		})

		// A credential that cannot be trusted always ends the request, even for a
		// public endpoint: quietly ignoring it would hide the caller's bug.
		It("refuses a request whose credential cannot be trusted", func() {
			reached := false
			mw := middleware.Auth(verifierStub{
				err: errors.New("bad token: " + authkit.ErrInvalidCredential.Error()),
			})

			res := mw(func(*rest.Request) *rest.Response {
				reached = true
				return &rest.Response{Code: http.StatusOK}
			})(authRequest())

			Expect(res.Code).To(Equal(http.StatusUnauthorized))
			Expect(reached).To(BeFalse(), "the handler must not run for a rejected credential")
		})

		It("answers a refusal in the shared error shape", func() {
			mw := middleware.Auth(verifierStub{err: errors.New("bad token")})

			res := mw(okHandler(&authkit.Principal{}))(authRequest())

			envelope, ok := res.Body.(*restkit.ErrorEnvelope)
			Expect(ok).To(BeTrue())
			Expect(envelope.Error.Code).To(Equal(restkit.UnauthenticatedCode))
			Expect(envelope.Error.Message).NotTo(BeEmpty())
		})

		// RFC 6750 asks a rejected bearer request to say which scheme it expects.
		It("tells the caller which authentication scheme to use", func() {
			mw := middleware.Auth(verifierStub{err: errors.New("bad token")})

			res := mw(okHandler(&authkit.Principal{}))(authRequest())

			Expect(res.AddHeaders).To(HaveKeyWithValue("WWW-Authenticate", "Bearer"))
		})

		// A dead cookie has to be taken back, and the only place to put a
		// Set-Cookie is the response the handler produced — which does not exist
		// yet when the verdict is reached.
		Describe("a verdict that asks the browser to drop a cookie", func() {
			discarding := func() rest.Middleware {
				return middleware.Auth(verifierStub{
					discard: []*http.Cookie{{Name: "__Host-session", MaxAge: -1}},
				})
			}

			It("puts the instruction on the handler's response", func() {
				res := discarding()(okHandler(&authkit.Principal{}))(authRequest())

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Cookies).To(HaveLen(1))
				Expect(res.Cookies[0].Name).To(Equal("__Host-session"))
			})

			// The request still runs: a dead cookie means anonymous, not refused.
			It("still serves the request", func() {
				reached := false

				discarding()(func(*rest.Request) *rest.Response {
					reached = true
					return &rest.Response{Code: http.StatusOK}
				})(authRequest())

				Expect(reached).To(BeTrue())
			})

			It("leaves the handler's own cookies alone", func() {
				res := discarding()(func(*rest.Request) *rest.Response {
					return &rest.Response{
						Code:    http.StatusOK,
						Cookies: []*http.Cookie{{Name: "theme", Value: "dark"}},
					}
				})(authRequest())

				Expect(res.Cookies).To(HaveLen(2))
			})

			// Some handlers answer with nothing, and there is nowhere to put a
			// Set-Cookie on that. The instruction is not lost for long: the
			// browser sends the same cookie with its next request.
			It("does not invent a response when the handler answered with none", func() {
				res := discarding()(func(*rest.Request) *rest.Response { return nil })(authRequest())

				Expect(res).To(BeNil())
			})
		})

		// The failure that must not look like a bad credential. A key store or a
		// session store that is down says nothing about the credential, and 401
		// would tell every caller in the system that theirs is bad at the moment
		// nothing can check any of them — which a browser acts on by signing out.
		Describe("when the credential could not be checked at all", func() {
			unavailable := func() rest.Middleware {
				return middleware.Auth(verifierStub{
					err: fmt.Errorf("%w: connection refused", authkit.ErrCredentialUnavailable),
				})
			}

			It("answers 503 rather than 401", func() {
				res := unavailable()(okHandler(&authkit.Principal{}))(authRequest())

				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
			})

			// Serving the request as nobody would quietly narrow what the caller
			// can do, instead of saying that nothing could be decided.
			It("does not serve the request anonymously", func() {
				reached := false

				res := unavailable()(func(*rest.Request) *rest.Response {
					reached = true
					return &rest.Response{Code: http.StatusOK}
				})(authRequest())

				Expect(reached).To(BeFalse())
				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
			})

			// Naming the dependency that is down tells a prober what to attack.
			It("says nothing about what failed", func() {
				res := unavailable()(okHandler(&authkit.Principal{}))(authRequest())

				envelope, ok := res.Body.(*restkit.ErrorEnvelope)
				Expect(ok).To(BeTrue())
				Expect(envelope.Error.Code).To(Equal(restkit.UnavailableCode))
				Expect(envelope.Error.Message).NotTo(ContainSubstring("connection refused"))
			})
		})
	})

	Describe("in front of a WebSocket upgrade", func() {
		It("lets the upgrade proceed and carries the caller into the connection", func() {
			guard := middleware.AuthUpgrade(verifierStub{
				principal: &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT},
			})
			r := authRequest()

			res := guard(r)

			Expect(res).To(BeNil(), "a nil response lets the upgrade happen")
			p, ok := authkit.PrincipalFrom(r.Context())
			Expect(ok).To(BeTrue())
			Expect(p.Subject).To(Equal("usr_1"))
		})

		It("lets an upgrade with no credential proceed", func() {
			guard := middleware.AuthUpgrade(verifierStub{})

			Expect(guard(authRequest())).To(BeNil())
		})

		// Rejecting before the upgrade means no connection is ever established.
		It("stops the upgrade when the credential cannot be trusted", func() {
			guard := middleware.AuthUpgrade(verifierStub{err: errors.New("bad token")})

			res := guard(authRequest())

			Expect(res).NotTo(BeNil())
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})

		// A connection is long-lived, so opening one on an unverified credential
		// would keep the doubt around for as long as it lasts.
		It("stops the upgrade when the credential could not be checked", func() {
			guard := middleware.AuthUpgrade(verifierStub{
				err: fmt.Errorf("%w: connection refused", authkit.ErrCredentialUnavailable),
			})

			res := guard(authRequest())

			Expect(res).NotTo(BeNil())
			Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})
