package middleware_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"
)

// handshakeAs is an upgrade request the way AuthUpgrade leaves it: carrying the
// caller when there is one.
func handshakeAs(caller *authkit.Principal) *rest.Request {
	r := authRequest()
	if caller != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), caller))
	}
	return r
}

// refusalCode reads the machine-readable code off a refusal.
func refusalCode(res *rest.Response) string {
	GinkgoHelper()

	envelope, ok := res.Body.(*restkit.ErrorEnvelope)
	Expect(ok).To(BeTrue(), "the refusal is not the common error envelope")
	return envelope.Error.Code
}

var _ = Describe("ChannelSecurity", func() {
	jwtCaller := func(scopes ...string) *authkit.Principal {
		return &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT, Scopes: scopes}
	}

	Describe("a channel with no declaration", func() {
		It("refuses an anonymous handshake with 401, in the shape every 401 has", func() {
			res := middleware.ChannelSecurity(nil)(handshakeAs(nil))

			Expect(res).NotTo(BeNil())
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
			Expect(refusalCode(res)).To(Equal(restkit.UnauthenticatedCode))
			Expect(res.AddHeaders).To(HaveKey("WWW-Authenticate"))
		})

		It("lets a verified caller upgrade", func() {
			Expect(middleware.ChannelSecurity(nil)(handshakeAs(jwtCaller()))).To(BeNil())
		})
	})

	It("lets anyone upgrade to a public channel", func() {
		Expect(middleware.ChannelSecurity(&wskit.SecuritySpec{Public: true})(handshakeAs(nil))).To(BeNil())
	})

	It("refuses a caller without a required scope with 403", func() {
		res := middleware.ChannelSecurity(&wskit.SecuritySpec{Scopes: []string{"things:read"}})(handshakeAs(jwtCaller()))

		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusForbidden))
		Expect(refusalCode(res)).To(Equal(restkit.ForbiddenCode))
	})

	It("refuses a caller who presented a kind of credential the channel does not accept with 403", func() {
		res := middleware.ChannelSecurity(&wskit.SecuritySpec{Schemes: []string{authkit.SchemeSession}})(handshakeAs(jwtCaller()))

		Expect(res).NotTo(BeNil())
		Expect(res.Code).To(Equal(http.StatusForbidden))
	})

	It("lets a caller with the scope upgrade", func() {
		Expect(middleware.ChannelSecurity(&wskit.SecuritySpec{Scopes: []string{"things:read"}})(handshakeAs(jwtCaller("things:read")))).To(BeNil())
	})
})
