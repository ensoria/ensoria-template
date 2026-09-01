package sessionkit_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

// writtenCookie renders a cookie the way it reaches a browser, so the specs
// assert on the header rather than on the struct that produced it.
func writtenCookie(cookie *http.Cookie) string {
	rec := httptest.NewRecorder()
	http.SetCookie(rec, cookie)
	return rec.Header().Get("Set-Cookie")
}

var _ = Describe("Cookies", func() {
	var cookies *sessionkit.Cookies

	BeforeEach(func() {
		cookies = sessionkit.NewCookies(testConfig())
	})

	Describe("Issue", func() {
		It("carries the session id under the configured name", func() {
			cookie := cookies.Issue(&sessionkit.Session{ID: "session-id"})

			Expect(cookie.Name).To(Equal("__Host-session"))
			Expect(cookie.Value).To(Equal("session-id"))
		})

		// None of these is a setting, and all three are what the __Host- prefix
		// makes the browser enforce anyway.
		It("is HttpOnly, site-wide, and bound to no domain", func() {
			cookie := cookies.Issue(&sessionkit.Session{ID: "session-id"})

			Expect(cookie.HttpOnly).To(BeTrue())
			Expect(cookie.Path).To(Equal("/"))
			Expect(cookie.Domain).To(BeEmpty())
			Expect(cookie.Secure).To(BeTrue())
			Expect(cookie.SameSite).To(Equal(http.SameSiteLaxMode))
		})

		// Without Max-Age the browser drops the cookie when it closes, which is
		// what makes the default profile a browser session.
		It("gives a default session no Max-Age at all", func() {
			cookie := cookies.Issue(&sessionkit.Session{ID: "session-id"})

			Expect(cookie.MaxAge).To(BeZero())
			Expect(writtenCookie(cookie)).NotTo(ContainSubstring("Max-Age"))
		})

		It("gives a persistent session the profile's own lifetime", func() {
			cookie := cookies.Issue(&sessionkit.Session{ID: "session-id", Persistent: true})

			Expect(cookie.MaxAge).To(Equal(int(testPersistentAbsoluteTTL / time.Second)))
		})
	})

	Describe("Discard", func() {
		// Max-Age=0 is the instruction to drop it now. Go writes that for a
		// negative MaxAge; zero means "no Max-Age attribute", which would leave
		// the cookie exactly where it is.
		It("tells the browser to drop the cookie now", func() {
			cookie := cookies.Discard()

			Expect(cookie.Value).To(BeEmpty())
			Expect(writtenCookie(cookie)).To(ContainSubstring("Max-Age=0"))
		})

		// A browser matches a cookie by name, path and domain. Discarding one
		// written under a different path replaces nothing and leaves the
		// original being sent with every request.
		It("matches the cookie it is meant to replace", func() {
			issued := cookies.Issue(&sessionkit.Session{ID: "session-id"})
			discard := cookies.Discard()

			Expect(discard.Name).To(Equal(issued.Name))
			Expect(discard.Path).To(Equal(issued.Path))
			Expect(discard.Domain).To(Equal(issued.Domain))
			Expect(discard.Secure).To(Equal(issued.Secure))
			Expect(discard.SameSite).To(Equal(issued.SameSite))
			Expect(discard.HttpOnly).To(Equal(issued.HttpOnly))
		})
	})

	Describe("Name", func() {
		It("is the name the verifier reads and the documentation publishes", func() {
			Expect(cookies.Name()).To(Equal("__Host-session"))
		})
	})
})
