package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/config/pkg/appconfig"
	authapi "github.com/ensoria/ensoria-template/internal/app/auth/api"
	sessionhttp "github.com/ensoria/ensoria-template/internal/app/auth/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/rest/pkg/pipeline"
	"github.com/ensoria/rest/pkg/rest"
)

// The specs here drive the whole cookie flow over real HTTP: a token is
// exchanged for a session, the session is used on its own, and it is given
// back. What each part does is tested in its own package; what this file
// checks is that a browser can actually sign in and out.

// The cookie the test server writes.
//
// It is not the default __Host- name because these servers speak plain HTTP:
// that prefix requires Secure, Secure keeps the cookie off an http:// URL, and
// a jar that refuses to store the cookie would test nothing. The default name
// is checked on its own, against the Set-Cookie header rather than through a
// jar, in "the cookie a deployment writes".
const e2eSessionCookie = "session"

// e2eTrustedOrigin is the origin the test server treats as its own frontend.
const e2eTrustedOrigin = "https://app.example.test"

// e2eSessionAuth is the configuration of a server that authenticates browsers.
func e2eSessionAuth() *appconfig.Auth {
	cfg := e2eAuth()
	cfg.Session = &appconfig.AuthSession{
		Store:                 appconfig.AuthSessionStoreMemory,
		CookieName:            e2eSessionCookie,
		CookieSameSite:        appconfig.AuthSessionSameSiteLax,
		CookieInsecure:        true,
		AbsoluteTTL:           time.Hour,
		PersistentAbsoluteTTL: 30 * 24 * time.Hour,
		IdleTTL:               24 * time.Hour,
	}
	return cfg
}

// e2eSessionStore is the store the exchange writes into.
func e2eSessionStore(cfg *appconfig.Auth) sessionkit.Store {
	GinkgoHelper()
	return e2eStoreOver(cfg, cachememory.New("e2e-session"))
}

// e2eStoreOver builds a session store over the given cache.
func e2eStoreOver(cfg *appconfig.Auth, cache enscache.Cache) sessionkit.Store {
	GinkgoHelper()
	sessionCfg, err := sessionkit.NewConfig(cfg.Session)
	Expect(err).NotTo(HaveOccurred())
	store, err := sessionkit.NewStore(cache, sessionCfg)
	Expect(err).NotTo(HaveOccurred())
	return store
}

// serveSessions starts a server that serves the session exchange, an endpoint
// any credential opens, and one only a session opens.
//
// allowOrigin is CORS_ALLOW_ORIGIN as a deployment writes it, and it feeds both
// the CORS middleware and the cross-origin check — which is the point of
// sharing one setting. Empty is the same-origin deployment: CORS has nothing to
// answer, and the cross-origin check is on its own.
func serveSessions(cfg *appconfig.Auth, store sessionkit.Store, allowOrigin string) *httptest.Server {
	GinkgoHelper()

	sessionCfg, err := sessionkit.NewConfig(cfg.Session)
	Expect(err).NotTo(HaveOccurred())
	cookies := sessionkit.NewCookies(sessionCfg)

	verifier, err := authkit.NewVerifier(cfg, nil, store)
	Expect(err).NotTo(HaveOccurred())

	origins := middleware.ParseOrigins(allowOrigin)
	crossOrigin, err := middleware.NewCrossOriginProtection(origins)
	Expect(err).NotTo(HaveOccurred())

	whoami := func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[e2eBody], error) {
		principal, _ := authkit.PrincipalFrom(r.Context())
		return rest.NewResult(&e2eBody{Name: principal.Subject}), nil
	}
	guarded := func(security *restkit.SecuritySpec) rest.Controller {
		return restkit.NewController(&restkit.Endpoint[restkit.NoBody, e2eBody]{
			Success: http.StatusOK, Security: security, Handle: whoami,
		})
	}

	httpPipeline := &pipeline.HTTP{
		Modules: []*rest.Module{
			authapi.NewSessionModule(store, cookies),
			{Path: "/private", Get: guarded(nil)},
			{Path: "/session-only", Get: guarded(&restkit.SecuritySpec{
				Schemes: []string{authkit.SchemeSession},
			})},
		},
		GlobalMiddlewares: globalMiddlewares(
			&appconfig.CORS{AllowOriginVal: allowOrigin, AllowCredentialsVal: allowOrigin != ""},
			crossOrigin,
			verifier,
			&rest.Response{Code: http.StatusInternalServerError},
		),
	}
	mux := http.NewServeMux()
	httpPipeline.Register(mux)
	return httptest.NewServer(mux)
}

// browser is a client that keeps cookies, which is the whole point here: the
// specs below check that a session survives from one request to the next
// without anybody carrying it by hand.
func browser() *http.Client {
	GinkgoHelper()
	jar, err := cookiejar.New(nil)
	Expect(err).NotTo(HaveOccurred())
	return &http.Client{Jar: jar}
}

// send performs one request and returns the whole response, so that a spec can
// look at its cookies as well as its body.
func send(client *http.Client, server *httptest.Server, method, path string,
	headers map[string]string, body string) (*http.Response, string) {
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

	res, err := client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = res.Body.Close() }()

	payload, err := io.ReadAll(res.Body)
	Expect(err).NotTo(HaveOccurred())
	return res, string(payload)
}

// setCookie returns the cookie of that name the response writes, or nil.
func setCookie(res *http.Response, name string) *http.Cookie {
	for _, cookie := range res.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// failingStore answers every call with an outage.
//
// ⚠ It reports a plain error, never sessionkit.ErrSessionNotFound. That is the
// distinction the whole design rests on: "gone" ends a session, "could not ask"
// must not.
type failingStore struct{ err error }

func (s *failingStore) Create(_ context.Context, _ *sessionkit.Snapshot, _ bool) (*sessionkit.Session, error) {
	return nil, s.err
}
func (s *failingStore) Lookup(context.Context, string) (*sessionkit.Session, error) {
	return nil, s.err
}
func (s *failingStore) Revoke(context.Context, string) error        { return s.err }
func (s *failingStore) RevokeSubject(context.Context, string) error { return s.err }

var _ = Describe("cookie authentication over HTTP", func() {
	var (
		cfg    *appconfig.Auth
		store  sessionkit.Store
		server *httptest.Server
		client *http.Client
	)

	BeforeEach(func() {
		cfg = e2eSessionAuth()
		store = e2eSessionStore(cfg)
		server = serveSessions(cfg, store, e2eTrustedOrigin)
		client = browser()
	})
	AfterEach(func() { server.Close() })

	// exchange trades the token for a session and returns the response.
	exchange := func(body string) (*http.Response, string) {
		GinkgoHelper()
		return send(client, server, http.MethodPost, "/session", bearer(""), body)
	}

	Describe("the whole round trip", func() {
		It("signs a browser in, serves it by cookie, and signs it out again", func() {
			res, body := exchange(`{}`)
			Expect(res.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				Subject   string    `json:"subject"`
				ExpiresAt time.Time `json:"expires_at"`
			}
			Expect(json.Unmarshal([]byte(body), &created)).To(Succeed())
			Expect(created.Subject).To(Equal("usr_1"))
			Expect(created.ExpiresAt).To(BeTemporally("~", time.Now().Add(time.Hour), time.Minute))

			// The request that follows carries no token at all: the cookie the
			// jar picked up is the only credential.
			res, body = send(client, server, http.MethodGet, "/session-only", nil, "")
			Expect(res.StatusCode).To(Equal(http.StatusOK))
			Expect(body).To(ContainSubstring("usr_1"))

			res, _ = send(client, server, http.MethodDelete, "/session", nil, "")
			Expect(res.StatusCode).To(Equal(http.StatusNoContent))

			res, _ = send(client, server, http.MethodGet, "/session-only", nil, "")
			Expect(res.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// The id is the credential. A response that repeats it hands it to any
		// script that can read a response body, which is what HttpOnly exists
		// to prevent.
		It("never puts the session id in the response body", func() {
			res, body := exchange(`{}`)

			cookie := setCookie(res, e2eSessionCookie)
			Expect(cookie).NotTo(BeNil())
			Expect(cookie.Value).NotTo(BeEmpty())
			Expect(body).NotTo(ContainSubstring(cookie.Value))
		})

		It("tells the browser to drop the cookie when the session ends", func() {
			exchange(`{}`)

			res, _ := send(client, server, http.MethodDelete, "/session", nil, "")

			cookie := setCookie(res, e2eSessionCookie)
			Expect(cookie).NotTo(BeNil())
			Expect(cookie.Value).To(BeEmpty())
			Expect(cookie.MaxAge).To(BeNumerically("<", 0))
		})

		// Signing out has to reach the server, not only the browser. A logout
		// that cleared the cookie and left the record alive would leave a
		// working credential behind for anyone who copied it.
		It("ends the session on the server, not only in the browser", func() {
			res, _ := exchange(`{}`)
			id := setCookie(res, e2eSessionCookie).Value

			send(client, server, http.MethodDelete, "/session", nil, "")

			_, err := store.Lookup(context.Background(), id)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})
	})

	Describe("the two lifetime profiles", func() {
		// Without Max-Age the browser drops the cookie when it closes, which is
		// what makes the default profile a browser session.
		It("writes a session cookie that does not outlive the browser", func() {
			res, _ := exchange(`{"persistent": false}`)

			cookie := setCookie(res, e2eSessionCookie)
			Expect(cookie.MaxAge).To(BeZero())
			Expect(cookie.Expires.IsZero()).To(BeTrue())
		})

		It("writes a cookie that survives the browser when asked to", func() {
			res, body := exchange(`{"persistent": true}`)

			cookie := setCookie(res, e2eSessionCookie)
			Expect(cookie.MaxAge).To(Equal(int((30 * 24 * time.Hour) / time.Second)))

			var created struct {
				Persistent bool      `json:"persistent"`
				ExpiresAt  time.Time `json:"expires_at"`
			}
			Expect(json.Unmarshal([]byte(body), &created)).To(Succeed())
			Expect(created.Persistent).To(BeTrue())
			// The longer profile, not the default one.
			Expect(created.ExpiresAt).To(BeTemporally(">", time.Now().Add(24*time.Hour)))
		})
	})

	Describe("the cookie a deployment writes", func() {
		// The attributes are checked here rather than through the jar above,
		// which cannot hold a Secure cookie sent over http://.
		It("is HttpOnly, Secure, SameSite=Lax and rooted at /", func() {
			secure := e2eSessionAuth()
			secure.Session.CookieName = appconfig.DefaultSessionCookieName
			secure.Session.CookieInsecure = false

			deployed := serveSessions(secure, e2eSessionStore(secure), e2eTrustedOrigin)
			defer deployed.Close()

			res, _ := send(&http.Client{}, deployed, http.MethodPost, "/session", bearer(""), `{}`)

			cookie := setCookie(res, appconfig.DefaultSessionCookieName)
			Expect(cookie).NotTo(BeNil())
			Expect(cookie.HttpOnly).To(BeTrue())
			Expect(cookie.Secure).To(BeTrue())
			Expect(cookie.SameSite).To(Equal(http.SameSiteLaxMode))
			Expect(cookie.Path).To(Equal("/"))
			Expect(cookie.Domain).To(BeEmpty())
		})
	})

	Describe("a token still beating a cookie", func() {
		// A cookie is attached by the browser; an Authorization header was put
		// there on purpose. Letting the cookie win would override what the
		// caller actually asked for.
		It("authenticates with the header when both are present", func() {
			exchange(`{}`)

			res, _ := send(client, server, http.MethodGet, "/session-only",
				map[string]string{"Authorization": "Bearer " + token("")}, "")

			// The token authenticates as the jwt scheme, which /session-only
			// does not accept: the cookie was not consulted.
			Expect(res.StatusCode).To(Equal(http.StatusForbidden))
		})
	})

	Describe("a second sign-in from the same browser", func() {
		It("ends the session it replaced", func() {
			res, _ := exchange(`{}`)
			first := setCookie(res, e2eSessionCookie).Value

			res, _ = exchange(`{}`)
			second := setCookie(res, e2eSessionCookie).Value
			Expect(second).NotTo(Equal(first))

			_, err := store.Lookup(context.Background(), first)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))

			_, err = store.Lookup(context.Background(), second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// The specs below run against a server with no CORS_ALLOW_ORIGIN at all —
	// the same-origin deployment, where the frontend is served by this
	// application. The CORS middleware has nothing to compare against there and
	// lets every origin through, so this check is the only thing standing
	// between a form on another site and an authenticated request.
	//
	// (With an origin configured, CORS refuses a foreign one first. That is a
	// second layer, not this one: it disappears the moment a deployment stops
	// needing CORS.)
	Describe("a cross-origin request to a same-origin deployment", func() {
		var (
			sameOrigin *httptest.Server
			signedIn   *http.Client
		)

		BeforeEach(func() {
			sameOrigin = serveSessions(cfg, store, "")
			signedIn = browser()
			res, _ := send(signedIn, sameOrigin, http.MethodPost, "/session", bearer(""), `{}`)
			Expect(res.StatusCode).To(Equal(http.StatusCreated))
		})
		AfterEach(func() { sameOrigin.Close() })

		It("refuses a state-changing request from an origin nobody trusts", func() {
			res, body := send(signedIn, sameOrigin, http.MethodDelete, "/session",
				map[string]string{"Origin": "https://evil.example"}, "")

			Expect(res.StatusCode).To(Equal(http.StatusForbidden))
			Expect(errorCode(body)).To(Equal(restkit.CrossOriginCode))
		})

		// ⚠ Documented rather than desired. The protection is built on safe
		// methods changing nothing, so it lets a cross-site GET through — with
		// the cookie attached. It is why a WebSocket upgrade, which is a GET,
		// needs an origin check of its own at the upgrade.
		It("lets a cross-site GET through, cookie and all", func() {
			res, body := send(signedIn, sameOrigin, http.MethodGet, "/session-only",
				map[string]string{"Origin": "https://evil.example"}, "")

			Expect(res.StatusCode).To(Equal(http.StatusOK))
			Expect(body).To(ContainSubstring("usr_1"))
		})

		// Anything that is not a browser sends neither Origin nor
		// Sec-Fetch-Site, and refusing those would break every
		// server-to-server caller in the system.
		It("leaves a caller that reports no origin alone", func() {
			res, _ := send(signedIn, sameOrigin, http.MethodDelete, "/session", nil, "")

			Expect(res.StatusCode).To(Equal(http.StatusNoContent))
		})
	})

	Describe("a cross-origin request to a deployment that names its frontend", func() {
		It("serves a state-changing request from the origin the configuration names", func() {
			res, _ := send(client, server, http.MethodPost, "/session",
				map[string]string{
					"Authorization": "Bearer " + token(""),
					"Origin":        e2eTrustedOrigin,
				}, `{}`)

			Expect(res.StatusCode).To(Equal(http.StatusCreated))
		})

		// ⚠ One layer refuses, not two. CORS tells the browser what it may
		// read and refuses nothing — a caller that is not a browser ignores it
		// anyway — so the refusal comes from the cross-origin check, with the
		// one error shape the rest of the API uses.
		It("refuses one from an origin nobody trusts", func() {
			res, body := send(client, server, http.MethodPost, "/session",
				map[string]string{
					"Authorization": "Bearer " + token(""),
					"Origin":        "https://evil.example",
				}, `{}`)

			Expect(res.StatusCode).To(Equal(http.StatusForbidden))
			Expect(errorCode(body)).To(Equal(restkit.CrossOriginCode))
			Expect(setCookie(res, e2eSessionCookie)).To(BeNil())
		})

		// ⚠ The half that used to be missing. A browser needs
		// Access-Control-Allow-Origin on the response it is going to read, not
		// only on the preflight — without it the sign-in succeeds on the server
		// and the frontend still cannot see the answer.
		It("puts the CORS headers on the sign-in response itself", func() {
			res, _ := send(client, server, http.MethodPost, "/session",
				map[string]string{
					"Authorization": "Bearer " + token(""),
					"Origin":        e2eTrustedOrigin,
				}, `{}`)

			Expect(res.StatusCode).To(Equal(http.StatusCreated))
			Expect(res.Header.Get("Access-Control-Allow-Origin")).To(Equal(e2eTrustedOrigin))
			// Without this the browser drops the cookie the response just set.
			Expect(res.Header.Get("Access-Control-Allow-Credentials")).To(Equal("true"))
			Expect(res.Header.Get("Vary")).To(ContainSubstring("Origin"))
		})
	})

	Describe("when the session store cannot be reached", func() {
		var outage *httptest.Server

		BeforeEach(func() {
			down := &failingStore{err: errors.New("dial tcp: connection refused")}
			outage = serveSessions(e2eSessionAuth(), down, e2eTrustedOrigin)
		})
		AfterEach(func() { outage.Close() })

		It("answers 503 to a sign-in and writes no cookie", func() {
			res, body := send(browser(), outage, http.MethodPost, "/session", bearer(""), `{}`)

			Expect(res.StatusCode).To(Equal(http.StatusServiceUnavailable))
			Expect(errorCode(body)).To(Equal(sessionhttp.CodeSessionNotCreated))
			Expect(res.Cookies()).To(BeEmpty())
		})

		// ⚠ The one that matters most. Answering 401 here would tell every
		// browser in the system that its perfectly good session is invalid, and
		// the discard instruction would sign all of them out at once — during
		// an outage, and they would not come back when it ended.
		It("answers 503 to a request carrying a cookie, and does not take it back", func() {
			request, err := http.NewRequest(http.MethodGet, outage.URL+"/session-only", nil)
			Expect(err).NotTo(HaveOccurred())
			request.AddCookie(&http.Cookie{Name: e2eSessionCookie, Value: "an-id-nobody-can-check"})

			res, err := http.DefaultClient.Do(request)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = res.Body.Close() }()

			Expect(res.StatusCode).To(Equal(http.StatusServiceUnavailable))
			Expect(res.Cookies()).To(BeEmpty())
		})
	})
})
