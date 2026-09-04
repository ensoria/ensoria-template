package http_test

import (
	"context"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/app/auth/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/app/auth/api/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/rest/pkg/rest"
)

// cookieName is the name the specs' sessions are carried under. It is not the
// __Host- default, which would drag the Secure rules in for no gain here.
const cookieName = "session"

// settings is the session configuration the specs run against.
func settings() *appconfig.AuthSession {
	return &appconfig.AuthSession{
		Store:                 appconfig.AuthSessionStoreMemory,
		CookieName:            cookieName,
		CookieSameSite:        appconfig.AuthSessionSameSiteLax,
		CookieInsecure:        true,
		AbsoluteTTL:           time.Hour,
		PersistentAbsoluteTTL: 30 * 24 * time.Hour,
		IdleTTL:               24 * time.Hour,
	}
}

// storeAndCookies builds a working store and the matching cookie writer.
func storeAndCookies() (sessionkit.Store, *sessionkit.Cookies) {
	GinkgoHelper()
	cfg, err := sessionkit.NewConfig(settings())
	Expect(err).NotTo(HaveOccurred())
	store, err := sessionkit.NewStore(cachememory.New("session-endpoints"), cfg)
	Expect(err).NotTo(HaveOccurred())
	return store, sessionkit.NewCookies(cfg)
}

// caller is the verified principal a request arrives with.
func caller() *authkit.Principal {
	return &authkit.Principal{
		Subject: "usr_1",
		Scopes:  []string{"things:read"},
		Scheme:  authkit.SchemeJWT,
		Claims:  map[string]any{"tenant": "acme"},
	}
}

// requestFrom builds a request carrying a verified caller, and the cookies the
// browser would have sent with it.
func requestFrom(principal *authkit.Principal, cookies ...*nethttp.Cookie) *rest.Request {
	raw := httptest.NewRequest(nethttp.MethodPost, "/session", nil)
	for _, cookie := range cookies {
		raw.AddCookie(cookie)
	}
	r := rest.NewRequest(raw)
	if principal != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), principal))
	}
	return r
}

// brokenStore reports an outage for everything.
//
// ⚠ Never sessionkit.ErrSessionNotFound: that one means "gone", and the whole
// design turns on the two not being confused.
type brokenStore struct{}

var errOutage = errors.New("dial tcp: connection refused")

func (brokenStore) Create(context.Context, *sessionkit.Snapshot, bool) (*sessionkit.Session, error) {
	return nil, errOutage
}
func (brokenStore) Lookup(context.Context, string) (*sessionkit.Session, error) {
	return nil, errOutage
}
func (brokenStore) Revoke(context.Context, string) error        { return errOutage }
func (brokenStore) RevokeSubject(context.Context, string) error { return errOutage }

// statusOf reads the status an endpoint's error carries.
func statusOf(err error) int {
	GinkgoHelper()
	var httpErr restkit.HTTPError
	Expect(errors.As(err, &httpErr)).To(BeTrue(), "the handler has to say which status it means")
	return httpErr.Status()
}

var _ = Describe("POST /session", func() {
	var (
		store   sessionkit.Store
		cookies *sessionkit.Cookies
	)

	BeforeEach(func() { store, cookies = storeAndCookies() })

	Describe("what it declares", func() {
		// The declaration is what the generated documentation and the startup
		// checks both read, so it is worth pinning rather than inferring.
		It("accepts a token and nothing else", func() {
			ep := http.NewCreateSession(store, cookies)

			Expect(ep.Security.Public).To(BeFalse())
			Expect(ep.Security.Schemes).To(Equal([]string{authkit.SchemeJWT}))
			// A caller asking to keep being who they already are needs no
			// permission beyond the one the token carries.
			Expect(ep.Security.Scopes).To(BeEmpty())
			Expect(ep.Success).To(Equal(nethttp.StatusCreated))
		})
	})

	It("creates a session for the caller and writes it as a cookie", func() {
		ep := http.NewCreateSession(store, cookies)

		result, err := ep.Handle(requestFrom(caller()), &dto.CreateSession{})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Body.Subject).To(Equal("usr_1"))
		Expect(result.Cookies).To(HaveLen(1))
		Expect(result.Cookies[0].Name).To(Equal(cookieName))
		Expect(result.Cookies[0].Value).NotTo(BeEmpty())
	})

	// The session outlives the token, so what the token said has to be copied
	// into it. A session that lost the caller's scopes would silently downgrade
	// them on the next request.
	It("keeps the caller's scopes and claims", func() {
		ep := http.NewCreateSession(store, cookies)
		result, err := ep.Handle(requestFrom(caller()), &dto.CreateSession{})
		Expect(err).NotTo(HaveOccurred())

		session, err := store.Lookup(context.Background(), result.Cookies[0].Value)

		Expect(err).NotTo(HaveOccurred())
		Expect(session.Snapshot.Scopes).To(Equal([]string{"things:read"}))
		Expect(session.Snapshot.Claims).To(HaveKeyWithValue("tenant", "acme"))
	})

	It("ends the session the same browser was already holding", func() {
		ep := http.NewCreateSession(store, cookies)
		first, err := ep.Handle(requestFrom(caller()), &dto.CreateSession{})
		Expect(err).NotTo(HaveOccurred())
		held := first.Cookies[0]

		second, err := ep.Handle(requestFrom(caller(), held), &dto.CreateSession{})
		Expect(err).NotTo(HaveOccurred())

		Expect(second.Cookies[0].Value).NotTo(Equal(held.Value))
		_, err = store.Lookup(context.Background(), held.Value)
		Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
	})

	Describe("when the store cannot be reached", func() {
		It("answers 503 and writes no cookie", func() {
			ep := http.NewCreateSession(brokenStore{}, cookies)

			result, err := ep.Handle(requestFrom(caller()), &dto.CreateSession{})

			Expect(result).To(BeNil())
			Expect(statusOf(err)).To(Equal(nethttp.StatusServiceUnavailable))
		})

		// The old session is unreachable from this browser and expires on its
		// own. Failing the sign-in over it would trade a working session for a
		// cleanup.
		It("still signs the caller in when only the replaced session survives", func() {
			working, writer := storeAndCookies()
			created, err := http.NewCreateSession(working, writer).
				Handle(requestFrom(caller()), &dto.CreateSession{})
			Expect(err).NotTo(HaveOccurred())

			// A store that creates but cannot revoke.
			ep := http.NewCreateSession(halfBrokenStore{Store: working}, writer)
			result, err := ep.Handle(requestFrom(caller(), created.Cookies[0]), &dto.CreateSession{})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Cookies[0].Value).NotTo(BeEmpty())
		})
	})

	// Unreachable while serving — the startup checks refuse it — but the
	// document generator builds this endpoint with nothing behind it.
	It("answers rather than panicking when there is no store at all", func() {
		ep := http.NewCreateSession(nil, nil)

		_, err := ep.Handle(requestFrom(caller()), &dto.CreateSession{})

		Expect(statusOf(err)).To(Equal(nethttp.StatusServiceUnavailable))
	})
})

// halfBrokenStore creates sessions and fails to end them.
type halfBrokenStore struct{ sessionkit.Store }

func (halfBrokenStore) Revoke(context.Context, string) error { return errOutage }

var _ = Describe("DELETE /session", func() {
	var (
		store   sessionkit.Store
		cookies *sessionkit.Cookies
	)

	BeforeEach(func() { store, cookies = storeAndCookies() })

	// signIn creates a session and returns the cookie the browser holds.
	signIn := func(store sessionkit.Store, cookies *sessionkit.Cookies) *nethttp.Cookie {
		GinkgoHelper()
		result, err := http.NewCreateSession(store, cookies).
			Handle(requestFrom(caller()), &dto.CreateSession{})
		Expect(err).NotTo(HaveOccurred())
		return result.Cookies[0]
	}

	Describe("what it declares", func() {
		It("accepts a session cookie and nothing else", func() {
			ep := http.NewDeleteSession(store, cookies)

			Expect(ep.Security.Public).To(BeFalse())
			Expect(ep.Security.Schemes).To(Equal([]string{authkit.SchemeSession}))
			Expect(ep.Success).To(Equal(nethttp.StatusNoContent))
		})
	})

	It("ends the session and tells the browser to drop the cookie", func() {
		held := signIn(store, cookies)
		ep := http.NewDeleteSession(store, cookies)

		result, err := ep.Handle(requestFrom(nil, held), nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Cookies).To(HaveLen(1))
		Expect(result.Cookies[0].Value).To(BeEmpty())
		Expect(result.Cookies[0].MaxAge).To(BeNumerically("<", 0))

		_, err = store.Lookup(context.Background(), held.Value)
		Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
	})

	// ⚠ The invariant worth pinning. Clearing the cookie while the session is
	// still alive throws away the only handle anyone had on it: the caller can
	// no longer end it, and believes they already have.
	It("keeps the cookie in place when the session could not be ended", func() {
		held := signIn(store, cookies)
		ep := http.NewDeleteSession(halfBrokenStore{Store: store}, cookies)

		result, err := ep.Handle(requestFrom(nil, held), nil)

		Expect(result).To(BeNil())
		Expect(statusOf(err)).To(Equal(nethttp.StatusServiceUnavailable))
	})

	It("answers rather than panicking when there is no store at all", func() {
		ep := http.NewDeleteSession(nil, nil)

		_, err := ep.Handle(requestFrom(nil), nil)

		Expect(statusOf(err)).To(Equal(nethttp.StatusServiceUnavailable))
	})
})
