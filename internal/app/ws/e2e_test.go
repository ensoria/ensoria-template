package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/config/pkg/appconfig"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// The cookie the specs' sessions travel in. Not the __Host- default, which
// would need Secure and so would never be sent to an http:// test server.
const e2eWSCookie = "session"

// e2eWSOrigin is the origin these specs' deployment calls its own frontend.
const e2eWSOrigin = "https://app.example.test"

// e2eWSPath is the channel the specs connect to.
const e2eWSPath = "/ws/things"

// wsAuth is the configuration of a deployment that authenticates browsers.
func wsAuth() *appconfig.Auth {
	return &appconfig.Auth{
		APIKeyHeader: appconfig.DefaultAPIKeyHeader,
		Session: &appconfig.AuthSession{
			Store:                 appconfig.AuthSessionStoreMemory,
			CookieName:            e2eWSCookie,
			CookieSameSite:        appconfig.AuthSessionSameSiteLax,
			CookieInsecure:        true,
			AbsoluteTTL:           time.Hour,
			PersistentAbsoluteTTL: 24 * time.Hour,
			IdleTTL:               time.Hour,
		},
	}
}

// openedAs is the caller a connection was opened for, as OnOpen saw it on the
// connection context. A nil principal means the connection is anonymous.
type openedAs struct {
	principal *authkit.Principal
	found     bool
}

// serveChannel starts a server carrying one channel, and reports who each
// connection was opened as.
//
// ⚠ It reads the principal from the context OnOpen receives, not from the
// handshake. That context is derived from the one the pre-upgrade middleware
// enriched, and whether the enrichment survives the upgrade is the thing worth
// checking — it is what lets a handler know who is on the other end.
func serveChannel(cfg *appconfig.Auth, origins *middleware.Origins, security *wskit.SecuritySpec) (*httptest.Server, sessionkit.Store, <-chan openedAs) {
	GinkgoHelper()

	sessionCfg, err := sessionkit.NewConfig(cfg.Session)
	Expect(err).NotTo(HaveOccurred())
	store, err := sessionkit.NewStore(cachememory.New("e2e-ws"), sessionCfg)
	Expect(err).NotTo(HaveOccurred())

	verifier, err := authkit.NewVerifier(cfg, nil, store)
	Expect(err).NotTo(HaveOccurred())

	opened := make(chan openedAs, 1)
	channel := &wskit.Channel{
		Path:     e2eWSPath,
		Security: security,
		Configure: func(m *wsconfig.Module) {
			m.OnOpen = func(ctx context.Context, _ *wsevent.Open) error {
				principal, found := authkit.PrincipalFrom(ctx)
				opened <- openedAs{principal: principal, found: found}
				return nil
			}
		},
	}

	// nil is the scope policy. This file drives a real connection to fix that a
	// caller reaches the lifecycle handlers at all; whether that caller carries
	// the policy is fixed in internal/middleware (AuthUpgrade), and the specs of
	// the user module's channel drive a message that only an implied scope
	// reaches.
	router := mustRouter(wsApp.CreateWSRouter([]*wskit.Module{wskit.NewModule(channel)}, verifier, origins, nil, nil))
	mux := http.NewServeMux()
	router.Register(mux)
	return httptest.NewServer(mux), store, opened
}

// dial opens a WebSocket connection, returning the handshake response so that a
// refusal can be inspected.
func dial(server *httptest.Server, headers http.Header) (*websocket.Conn, *http.Response, error) {
	url := "ws" + strings.TrimPrefix(server.URL, "http") + e2eWSPath
	return websocket.DefaultDialer.Dial(url, headers)
}

// signIn creates a session and returns the cookie header a browser would send.
func signIn(store sessionkit.Store) http.Header {
	GinkgoHelper()
	session, err := store.Create(context.Background(),
		&sessionkit.Snapshot{Subject: "usr_1", Scopes: []string{"things:read"}}, false)
	Expect(err).NotTo(HaveOccurred())

	headers := http.Header{}
	headers.Set("Cookie", e2eWSCookie+"="+session.ID)
	return headers
}

var _ = Describe("a WebSocket upgrade", func() {
	var (
		server *httptest.Server
		store  sessionkit.Store
		opened <-chan openedAs
	)

	// A public channel: these specs are about what reaches a connection —
	// the caller, the origin check, a dead cookie — on a channel that admits
	// anonymous callers too. What a channel requiring a caller refuses is below.
	BeforeEach(func() {
		server, store, opened = serveChannel(wsAuth(), middleware.ParseOrigins(e2eWSOrigin), &wskit.SecuritySpec{Public: true})
	})
	AfterEach(func() { server.Close() })

	// The reason cookies matter for WebSocket at all: the browser's WebSocket
	// API cannot set an Authorization header, so a cookie is the only credential
	// a browser can present at the handshake without inventing a scheme.
	Describe("carrying a session cookie", func() {
		It("opens the connection", func() {
			conn, res, err := dial(server, signIn(store))

			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()
			Expect(res.StatusCode).To(Equal(http.StatusSwitchingProtocols))
		})

		// ⚠ The propagation this pins. The pre-upgrade middleware writes the
		// caller onto the request context, and wsserver carries that context
		// across the upgrade; if it ever stopped doing so, every handler would
		// silently see an anonymous connection instead of failing.
		It("carries the caller onto the connection context", func() {
			conn, _, err := dial(server, signIn(store))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()

			var seen openedAs
			Eventually(opened).Should(Receive(&seen))
			Expect(seen.found).To(BeTrue())
			Expect(seen.principal.Subject).To(Equal("usr_1"))
			Expect(seen.principal.Scopes).To(Equal([]string{"things:read"}))
			// The session scheme, whatever was presented when it was created.
			Expect(seen.principal.Scheme).To(Equal(authkit.SchemeSession))
		})
	})

	Describe("from another origin", func() {
		fromOrigin := func(origin string, headers http.Header) http.Header {
			headers.Set("Origin", origin)
			return headers
		}

		// ⚠ Cross-site WebSocket hijacking. The same-origin policy does not
		// apply to WebSocket connections, so without this a page on any site
		// could open one with the user's cookie attached — and read everything
		// it carries, which an ordinary forged request cannot do.
		It("is refused when the origin is not one the deployment claims", func() {
			_, res, err := dial(server, fromOrigin("https://evil.example", signIn(store)))

			Expect(err).To(HaveOccurred())
			Expect(res.StatusCode).To(Equal(http.StatusForbidden))
			Consistently(opened).ShouldNot(Receive())
		})

		It("is served when the origin is the one the configuration names", func() {
			conn, res, err := dial(server, fromOrigin(e2eWSOrigin, signIn(store)))

			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()
			Expect(res.StatusCode).To(Equal(http.StatusSwitchingProtocols))
		})
	})

	// ⚠ Documented rather than desired. The upgrade guard refuses a credential
	// it cannot trust, but an absent one leaves the connection anonymous — the
	// channel's declaration decides whether that is acceptable, and this one is
	// public. A dead cookie is the same case: the verifier reports it as
	// anonymous, and the discard instruction it produced has nowhere to go,
	// because the WebSocket library writes the handshake response itself.
	Describe("carrying a cookie that no longer resolves", func() {
		It("opens an anonymous connection rather than refusing", func() {
			headers := http.Header{}
			headers.Set("Cookie", e2eWSCookie+"=an-id-that-was-never-issued")

			conn, res, err := dial(server, headers)

			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()
			Expect(res.StatusCode).To(Equal(http.StatusSwitchingProtocols))
			// And the browser keeps the dead cookie: there is no Set-Cookie to
			// send. Its next ordinary HTTP request carries the instruction.
			Expect(res.Header.Get("Set-Cookie")).To(BeEmpty())

			var seen openedAs
			Eventually(opened).Should(Receive(&seen))
			Expect(seen.found).To(BeFalse())
		})
	})
})

// The credentials of a deployment that verifies every kind: a token signed with
// the shared secret, an API key from the configuration, and a session cookie.
const (
	e2eWSSecret = "e2e-ws-shared-secret"
	e2eWSAPIKey = "e2e-ws-api-key"
	// e2eWSTokenLifetime is how long the specs' tokens are valid.
	e2eWSTokenLifetime = time.Hour
)

// wsAuthAllSchemes verifies tokens, API keys and session cookies.
func wsAuthAllSchemes() *appconfig.Auth {
	cfg := wsAuth()
	cfg.Mode = appconfig.AuthModeHS256
	cfg.Secret = e2eWSSecret
	cfg.APIKeys = []string{e2eWSAPIKey}
	return cfg
}

// bearer is the header a machine caller sends: a token carrying scopes.
func bearer(scopes ...string) http.Header {
	GinkgoHelper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "usr_token",
		"exp":   time.Now().Add(e2eWSTokenLifetime).Unix(),
		"scope": strings.Join(scopes, " "),
	}).SignedString([]byte(e2eWSSecret))
	Expect(err).NotTo(HaveOccurred())

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	return headers
}

// apiKey is the header a server caller sends. Keys from the configuration carry
// no scopes.
func apiKey() http.Header {
	headers := http.Header{}
	headers.Set(appconfig.DefaultAPIKeyHeader, e2eWSAPIKey)
	return headers
}

// sessionWith signs a browser in with the given scopes.
func sessionWith(store sessionkit.Store, scopes ...string) http.Header {
	GinkgoHelper()

	session, err := store.Create(context.Background(), &sessionkit.Snapshot{Subject: "usr_1", Scopes: scopes}, false)
	Expect(err).NotTo(HaveOccurred())
	headers := http.Header{}
	headers.Set("Cookie", e2eWSCookie+"="+session.ID)
	return headers
}

// refusedWith dials and asserts the handshake was refused with status, before
// any connection was opened.
func refusedWith(server *httptest.Server, headers http.Header, opened <-chan openedAs, status int) {
	GinkgoHelper()

	conn, res, err := dial(server, headers)
	if conn != nil {
		_ = conn.Close()
	}
	Expect(err).To(HaveOccurred())
	Expect(res).NotTo(BeNil())
	Expect(res.StatusCode).To(Equal(status))
	Consistently(opened).ShouldNot(Receive())
}

// openedFor dials and asserts the connection opened, returning who it opened as.
func openedFor(server *httptest.Server, headers http.Header, opened <-chan openedAs) openedAs {
	GinkgoHelper()

	conn, res, err := dial(server, headers)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = conn.Close() })
	Expect(res.StatusCode).To(Equal(http.StatusSwitchingProtocols))
	var seen openedAs
	Eventually(opened).Should(Receive(&seen))
	return seen
}

// Before the upgrade, while the handshake is an HTTP request: a refused caller
// gets the 401 or 403 an HTTP client already knows how to handle, and no
// connection is ever opened.
var _ = Describe("a WebSocket upgrade to a channel that requires a caller", func() {
	var (
		server *httptest.Server
		store  sessionkit.Store
		opened <-chan openedAs
	)
	serve := func(security *wskit.SecuritySpec) {
		server, store, opened = serveChannel(wsAuthAllSchemes(), middleware.ParseOrigins(e2eWSOrigin), security)
		DeferCleanup(server.Close)
	}

	Describe("declaring nothing", func() {
		BeforeEach(func() { serve(nil) })

		It("refuses an anonymous handshake with 401", func() {
			refusedWith(server, http.Header{}, opened, http.StatusUnauthorized)
		})

		// On a public channel a dead cookie is just an anonymous caller; here
		// that is a caller the channel does not admit.
		It("refuses a cookie that no longer resolves with 401", func() {
			headers := http.Header{}
			headers.Set("Cookie", e2eWSCookie+"=an-id-that-was-never-issued")

			refusedWith(server, headers, opened, http.StatusUnauthorized)
		})

		It("opens a connection for a caller with a session", func() {
			Expect(openedFor(server, sessionWith(store), opened).principal.Scheme).To(Equal(authkit.SchemeSession))
		})

		It("opens a connection for a caller with a token", func() {
			Expect(openedFor(server, bearer(), opened).principal.Scheme).To(Equal(authkit.SchemeJWT))
		})

		It("opens a connection for a caller with an API key", func() {
			Expect(openedFor(server, apiKey(), opened).principal.Scheme).To(Equal(authkit.SchemeAPIKey))
		})
	})

	Describe("requiring a scope", func() {
		BeforeEach(func() { serve(&wskit.SecuritySpec{Scopes: []string{"things:read"}}) })

		It("refuses an anonymous handshake with 401", func() {
			refusedWith(server, http.Header{}, opened, http.StatusUnauthorized)
		})

		It("refuses a session without the scope with 403", func() {
			refusedWith(server, sessionWith(store), opened, http.StatusForbidden)
		})

		It("opens a connection for a session with the scope", func() {
			openedFor(server, sessionWith(store, "things:read"), opened)
		})

		It("refuses a token without the scope with 403", func() {
			refusedWith(server, bearer(), opened, http.StatusForbidden)
		})

		It("opens a connection for a token with the scope", func() {
			openedFor(server, bearer("things:read"), opened)
		})

		// Keys from the configuration carry no scopes at all.
		It("refuses an API key without the scope with 403", func() {
			refusedWith(server, apiKey(), opened, http.StatusForbidden)
		})
	})

	Describe("accepting only API keys", func() {
		BeforeEach(func() { serve(&wskit.SecuritySpec{Schemes: []string{authkit.SchemeAPIKey}}) })

		It("opens a connection for an API key", func() {
			openedFor(server, apiKey(), opened)
		})

		It("refuses a token with 403", func() {
			refusedWith(server, bearer(), opened, http.StatusForbidden)
		})

		It("refuses a session with 403", func() {
			refusedWith(server, sessionWith(store), opened, http.StatusForbidden)
		})
	})
})
