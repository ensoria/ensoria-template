package middleware_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// handshake builds a WebSocket upgrade addressed to this host, said to come
// from origin ("" sends no Origin header, as a non-browser client does).
func handshake(origin string) *rest.Request {
	req := httptest.NewRequest(http.MethodGet, siteHost+"/ws/things", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return rest.NewRequest(req)
}

var _ = Describe("UpgradeOrigin", func() {
	// The deployment whose frontend is somewhere else.
	separate := middleware.ParseOrigins("https://app.example.test")
	// The deployment that serves its own frontend and configures no CORS.
	sameSite := middleware.ParseOrigins("")

	// ⚠ The attack this exists for. An upgrade is a GET, so the cross-origin
	// check lets it through; the same-origin policy does not apply to WebSocket
	// connections at all, so without this a page on any site could open one
	// with the user's cookie attached and read everything it carries.
	It("refuses a handshake a browser started from an origin nobody claims", func() {
		res := middleware.UpgradeOrigin(separate)(handshake("https://evil.example"))

		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusForbidden))
		envelope, ok := res.Body.(*restkit.ErrorEnvelope)
		Expect(ok).To(BeTrue())
		Expect(envelope.Error.Code).To(Equal(restkit.CrossOriginCode))
	})

	It("allows the origin the configuration names", func() {
		Expect(middleware.UpgradeOrigin(separate)(handshake("https://app.example.test"))).To(BeNil())
	})

	// The whole point of comparing against the host as well: a deployment that
	// serves its own frontend configures no origins at all, and its own pages
	// still have to be able to connect.
	It("allows a page served by this same host, with nothing configured", func() {
		Expect(middleware.UpgradeOrigin(sameSite)(handshake(siteHost))).To(BeNil())
	})

	It("still refuses another origin when nothing is configured", func() {
		Expect(middleware.UpgradeOrigin(sameSite)(handshake("https://evil.example"))).NotTo(BeNil())
	})

	// RFC 6455 requires browsers to send Origin. A handshake without one did
	// not come from a browser, so no cookie was attached without its owner
	// meaning it — and refusing would break every non-browser client.
	It("allows a handshake that carries no origin at all", func() {
		Expect(middleware.UpgradeOrigin(separate)(handshake(""))).To(BeNil())
	})

	It("allows everything when every origin is claimed", func() {
		wildcard := middleware.ParseOrigins("*")

		Expect(middleware.UpgradeOrigin(wildcard)(handshake("https://anywhere.example"))).To(BeNil())
	})

	It("refuses an Origin that is not a URL", func() {
		Expect(middleware.UpgradeOrigin(separate)(handshake("://nonsense"))).NotTo(BeNil())
	})

	// A GET is exactly what an upgrade is, and the cross-origin check lets
	// every GET through. Pinning both halves here is what keeps someone from
	// concluding that one check covers the other.
	It("catches what the cross-origin check deliberately does not", func() {
		protection, err := middleware.NewCrossOriginProtection(separate)
		Expect(err).NotTo(HaveOccurred())

		forged := handshake("https://evil.example")

		Expect(protection.Check(forged.Underlying())).To(Succeed(), "a GET is always allowed there")
		Expect(middleware.UpgradeOrigin(separate)(forged)).NotTo(BeNil(), "but not here")
	})
})
