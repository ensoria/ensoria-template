package authkit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/rest/pkg/rest"
)

// errStoreDown stands in for the session store being unreachable.
var errStoreDown = errors.New("connection refused")

// sessionStore answers lookups with whatever a spec set, so the verifier can be
// exercised without a Redis.
type sessionStore struct {
	sessionkit.Store
	session *sessionkit.Session
	err     error
}

func (s sessionStore) Lookup(context.Context, string) (*sessionkit.Session, error) {
	return s.session, s.err
}

// sessionConfig is the session half of a configuration that turns cookies on.
func sessionConfig() *appconfig.AuthSession {
	return &appconfig.AuthSession{
		Store:                 appconfig.AuthSessionStoreRedis,
		CookieName:            appconfig.DefaultSessionCookieName,
		CookieSameSite:        appconfig.AuthSessionSameSiteLax,
		AbsoluteTTL:           appconfig.DefaultSessionAbsoluteTTL,
		PersistentAbsoluteTTL: appconfig.DefaultSessionPersistentAbsoluteTTL,
		IdleTTL:               appconfig.DefaultSessionIdleTTL,
	}
}

// requestWithCookie builds a request carrying a session cookie, and whatever
// headers a spec wants alongside it.
func requestWithCookie(value string, headers map[string]string) *rest.Request {
	raw := httptest.NewRequest(http.MethodGet, "/things", nil)
	if value != "" {
		raw.AddCookie(&http.Cookie{Name: appconfig.DefaultSessionCookieName, Value: value})
	}
	for name, header := range headers {
		raw.Header.Set(name, header)
	}
	return rest.NewRequest(raw)
}

// liveSession is a session the store resolves successfully.
func liveSession() *sessionkit.Session {
	return &sessionkit.Session{
		ID: "session-id",
		Snapshot: &sessionkit.Snapshot{
			Subject: "usr_1",
			Scopes:  []string{"orders:read"},
			Claims:  map[string]any{"org": "acme"},
		},
	}
}

var _ = Describe("the session cookie", func() {
	// verifierWith builds a verifier that accepts tokens and session cookies.
	verifierWith := func(store sessionkit.Store) authkit.Verifier {
		GinkgoHelper()

		v, err := authkit.NewVerifier(&appconfig.Auth{
			Mode: appconfig.AuthModeHS256, Secret: "secret", Session: sessionConfig(),
		}, nil, store)
		Expect(err).NotTo(HaveOccurred())
		return v
	}

	It("resolves the caller the session was created for", func() {
		v := verifierWith(sessionStore{session: liveSession()})

		result, err := v.Verify(requestWithCookie("session-id", nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Principal.Subject).To(Equal("usr_1"))
		Expect(result.Principal.Scopes).To(Equal([]string{"orders:read"}))
		Expect(result.Principal.Claims).To(HaveKeyWithValue("org", "acme"))
		Expect(result.Discard).To(BeEmpty())
	})

	// The snapshot was taken from a caller holding a token; this request holds a
	// cookie. An endpoint declaring Schemes: [session] is asking about this
	// request, so the scheme must not survive the trade.
	It("reports the caller as having authenticated with a session", func() {
		v := verifierWith(sessionStore{session: liveSession()})

		result, err := v.Verify(requestWithCookie("session-id", nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Principal.Scheme).To(Equal(authkit.SchemeSession))
	})

	It("carries no caller when the request has no cookie", func() {
		v := verifierWith(sessionStore{session: liveSession()})

		result, err := v.Verify(requestWithCookie("", nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Principal).To(BeNil())
		Expect(result.Discard).To(BeEmpty())
	})

	Describe("a cookie for a session that is gone", func() {
		gone := sessionStore{err: sessionkit.ErrSessionNotFound}

		// Unlike a bad token, which ends the request at 401 even on a public
		// endpoint. A browser sends a cookie without anyone deciding to, so a
		// dead one means the request is simply anonymous.
		It("is treated as no credential rather than a refusal", func() {
			result, err := verifierWith(gone).Verify(requestWithCookie("stale", nil))

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Principal).To(BeNil())
		})

		// Without this the browser presents the same dead cookie on every
		// request until it expires, which for the persistent profile is weeks.
		It("tells the browser to drop the cookie", func() {
			result, err := verifierWith(gone).Verify(requestWithCookie("stale", nil))

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Discard).To(HaveLen(1))
			Expect(result.Discard[0].Name).To(Equal(appconfig.DefaultSessionCookieName))
			Expect(result.Discard[0].Value).To(BeEmpty())
			Expect(result.Discard[0].MaxAge).To(BeNumerically("<", 0),
				"a negative MaxAge is what net/http writes as Max-Age=0")
		})
	})

	// The most damaging thing this path can get wrong. Telling every browser to
	// drop its cookie because the store is unreachable signs out every user at
	// once, and they do not come back when it recovers.
	Describe("a store that cannot be reached", func() {
		down := sessionStore{err: errStoreDown}

		It("is not treated as a bad credential", func() {
			_, err := verifierWith(down).Verify(requestWithCookie("session-id", nil))

			Expect(errors.Is(err, authkit.ErrCredentialUnavailable)).To(BeTrue())
			Expect(errors.Is(err, authkit.ErrInvalidCredential)).To(BeFalse())
		})

		It("never asks the browser to drop its cookie", func() {
			result, err := verifierWith(down).Verify(requestWithCookie("session-id", nil))

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil(), "a failed verdict carries no instructions")
		})
	})

	// A browser attaches a cookie to every request whether or not anyone meant
	// to send it; a header was put there on purpose. Letting the cookie win
	// would let whatever the browser is holding override what the caller asked
	// for.
	Describe("precedence", func() {
		It("prefers a bearer token over the cookie", func() {
			v := verifierWith(sessionStore{session: liveSession()})
			r := requestWithCookie("session-id", map[string]string{
				"Authorization": "Bearer not-a-real-token",
			})

			_, err := v.Verify(r)

			// The token is rubbish, so this is a refusal — which proves the
			// cookie was never consulted.
			Expect(errors.Is(err, authkit.ErrInvalidCredential)).To(BeTrue())
		})

		It("prefers an API key over the cookie", func() {
			v, err := authkit.NewVerifier(&appconfig.Auth{
				APIKeyHeader: appconfig.DefaultAPIKeyHeader,
				APIKeys:      []string{"a-key"},
				Session:      sessionConfig(),
			}, nil, sessionStore{session: liveSession()})
			Expect(err).NotTo(HaveOccurred())
			r := requestWithCookie("session-id", map[string]string{
				appconfig.DefaultAPIKeyHeader: "a-key",
			})

			result, err := v.Verify(r)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Principal.Scheme).To(Equal(authkit.SchemeAPIKey))
		})
	})

	Describe("what the verifier reports it can check", func() {
		It("includes the session scheme when a store was given", func() {
			v := verifierWith(sessionStore{})

			Expect(v.Schemes()).To(ContainElement(authkit.SchemeSession))
		})

		It("leaves it out when sessions are not configured", func() {
			v, err := authkit.NewVerifier(
				&appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: "s"}, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(v.Schemes()).NotTo(ContainElement(authkit.SchemeSession))
		})
	})

	// Both halves of the wiring have to be present. Either one alone is a
	// mistake that would otherwise surface much later, as endpoints declaring
	// Schemes: [session] failing a startup check about a scheme nothing
	// verifies — a description of the symptom rather than the cause.
	Describe("a half-wired configuration", func() {
		It("refuses a store with the selector unset", func() {
			_, err := authkit.NewVerifier(
				&appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: "s"},
				nil, sessionStore{})

			Expect(err).To(MatchError(ContainSubstring("AUTH_SESSION_STORE")))
		})

		It("refuses the selector with no store", func() {
			_, err := authkit.NewVerifier(
				&appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: "s", Session: sessionConfig()},
				nil, nil)

			Expect(err).To(MatchError(ContainSubstring("session store")))
		})

		// The cookie cannot be stored by any browser, so every sign-in would
		// appear to succeed and then not have happened.
		It("refuses a __Host- cookie that cannot carry Secure", func() {
			session := sessionConfig()
			session.CookieInsecure = true

			_, err := authkit.NewVerifier(
				&appconfig.Auth{Mode: appconfig.AuthModeHS256, Secret: "s", Session: session},
				nil, sessionStore{})

			Expect(err).To(MatchError(ContainSubstring("__Host-")))
		})
	})
})

var _ = Describe("SnapshotOf and PrincipalOf", func() {
	It("carries the caller's identity and permissions across the trade", func() {
		principal := &authkit.Principal{
			Subject: "usr_1",
			Scopes:  []string{"orders:read"},
			Scheme:  authkit.SchemeJWT,
			Claims:  map[string]any{"org": "acme"},
		}

		restored := authkit.PrincipalOf(authkit.SnapshotOf(principal))

		Expect(restored.Subject).To(Equal("usr_1"))
		Expect(restored.Scopes).To(Equal([]string{"orders:read"}))
		Expect(restored.Claims).To(HaveKeyWithValue("org", "acme"))
	})

	// The one field that must not round-trip: the snapshot was taken from a
	// caller holding a token, and everything that restores it holds a cookie.
	It("does not carry the scheme across the trade", func() {
		principal := &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT}

		restored := authkit.PrincipalOf(authkit.SnapshotOf(principal))

		Expect(restored.Scheme).To(Equal(authkit.SchemeSession))
	})

	// A caller holding the principal must not be able to widen what a live
	// session may do through it.
	It("copies rather than shares the permissions", func() {
		principal := &authkit.Principal{Subject: "usr_1", Scopes: []string{"orders:read"}}

		snapshot := authkit.SnapshotOf(principal)
		principal.Scopes[0] = "orders:write"

		Expect(authkit.PrincipalOf(snapshot).Scopes).To(Equal([]string{"orders:read"}))
	})

	It("answers nil for nil", func() {
		Expect(authkit.SnapshotOf(nil)).To(BeNil())
		Expect(authkit.PrincipalOf(nil)).To(BeNil())
	})
})
