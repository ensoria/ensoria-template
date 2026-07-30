package http

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// moduleWith builds one module per declaration, each on its own path.
func moduleWith(securities ...*restkit.SecuritySpec) []*rest.Module {
	modules := make([]*rest.Module, 0, len(securities))
	for i, security := range securities {
		ep := &restkit.Endpoint[restkit.NoBody, restkit.NoBody]{
			Security: security,
			Success:  http.StatusNoContent,
			Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[restkit.NoBody], error) {
				return restkit.NoContent(), nil
			},
		}
		modules = append(modules, &rest.Module{
			Path: fmt.Sprintf("/things/%d", i),
			Get:  restkit.NewController(ep),
		})
	}
	return modules
}

var _ = Describe("checkAuthConfiguration", func() {
	sharedSecret := &appconfig.Auth{Secret: "a-secret"}
	apiKeys := &appconfig.Auth{APIKeys: []string{"a-key"}}

	// An application whose endpoints are all public needs no configuration.
	It("accepts an unconfigured application whose endpoints are all public", func() {
		modules := moduleWith(&restkit.SecuritySpec{Public: true})

		Expect(checkAuthConfiguration(modules, &appconfig.Auth{})).To(Succeed())
	})

	It("refuses an unconfigured application whose endpoints need a caller", func() {
		err := checkAuthConfiguration(moduleWith(nil), &appconfig.Auth{})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("AUTH_SECRET"))
	})

	It("accepts endpoints that need a caller once anything can verify one", func() {
		Expect(checkAuthConfiguration(moduleWith(nil), sharedSecret)).To(Succeed())
	})

	Describe("an endpoint that insists on one kind of credential", func() {
		apiKeyOnly := &restkit.SecuritySpec{Schemes: []string{authkit.SchemeAPIKey}}

		// Requiring API keys while only JWTs are verifiable closes the endpoint
		// to everyone: no caller can ever present an accepted credential.
		It("is refused when nothing verifies that kind", func() {
			err := checkAuthConfiguration(moduleWith(apiKeyOnly), sharedSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(authkit.SchemeAPIKey))
		})

		It("names what is configured so the mismatch is visible", func() {
			err := checkAuthConfiguration(moduleWith(apiKeyOnly), sharedSecret)

			Expect(err.Error()).To(ContainSubstring(authkit.SchemeJWT))
		})

		It("is accepted once that kind is configured", func() {
			Expect(checkAuthConfiguration(moduleWith(apiKeyOnly), apiKeys)).To(Succeed())
		})

		// A public endpoint is served without any credential, so naming a
		// scheme on it cannot lock anyone out.
		//
		// The set also holds an endpoint that needs a caller, otherwise the
		// check would stop before ever looking at the schemes.
		It("is ignored on a public endpoint", func() {
			public := &restkit.SecuritySpec{Public: true, Schemes: []string{authkit.SchemeAPIKey}}

			Expect(checkAuthConfiguration(moduleWith(public, nil), sharedSecret)).To(Succeed())
		})
	})

	It("accepts an endpoint that names no scheme, whatever is configured", func() {
		wide := &restkit.SecuritySpec{Scopes: []string{"things:read"}}

		Expect(checkAuthConfiguration(moduleWith(wide), apiKeys)).To(Succeed())
	})
})
