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
func serveChannel(cfg *appconfig.Auth, origins *middleware.Origins) (*httptest.Server, sessionkit.Store, <-chan openedAs) {
	GinkgoHelper()

	sessionCfg, err := sessionkit.NewConfig(cfg.Session)
	Expect(err).NotTo(HaveOccurred())
	store, err := sessionkit.NewStore(cachememory.New("e2e-ws"), sessionCfg)
	Expect(err).NotTo(HaveOccurred())

	verifier, err := authkit.NewVerifier(cfg, nil, store)
	Expect(err).NotTo(HaveOccurred())

	opened := make(chan openedAs, 1)
	channel := &wskit.Channel{
		Path: e2eWSPath,
		Configure: func(m *wsconfig.Module) {
			m.OnOpen = func(ctx context.Context, _ *wsevent.Open) error {
				principal, found := authkit.PrincipalFrom(ctx)
				opened <- openedAs{principal: principal, found: found}
				return nil
			}
		},
	}

	router := wsApp.CreateWSRouter([]*wskit.Module{wskit.NewModule(channel)}, verifier, origins)
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

	BeforeEach(func() {
		server, store, opened = serveChannel(wsAuth(), middleware.ParseOrigins(e2eWSOrigin))
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
	// channel itself decides whether that is acceptable. A dead cookie is the
	// same case: the verifier reports it as anonymous, and the discard
	// instruction it produced has nowhere to go, because the WebSocket library
	// writes the handshake response itself.
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
