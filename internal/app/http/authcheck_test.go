package http

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

// verifierFor reports the credential kinds a verifier can check. What it does
// with a request does not matter here; the check only asks what it can verify.
type verifierFor []string

func (verifierFor) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return &authkit.VerifyResult{}, nil
}

func (v verifierFor) Schemes() []string { return v }

var _ = Describe("checkAuthConfiguration", func() {
	// An application verifying tokens only, and one verifying API keys only.
	tokensOnly := verifierFor{authkit.SchemeJWT}
	keysOnly := verifierFor{authkit.SchemeAPIKey}
	verifiesNothing := verifierFor{}

	// An application whose endpoints are all public needs no configuration.
	It("accepts an unconfigured application whose endpoints are all public", func() {
		modules := moduleWith(&restkit.SecuritySpec{Public: true})

		Expect(checkAuthConfiguration(modules, verifiesNothing)).To(Succeed())
	})

	It("refuses an application whose endpoints need a caller it cannot verify", func() {
		err := checkAuthConfiguration(moduleWith(nil), verifiesNothing)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("AUTH_SECRET"))
	})

	It("refuses an application with no verifier at all", func() {
		Expect(checkAuthConfiguration(moduleWith(nil), nil)).To(HaveOccurred())
	})

	It("accepts endpoints that need a caller once anything can verify one", func() {
		Expect(checkAuthConfiguration(moduleWith(nil), tokensOnly)).To(Succeed())
	})

	// A key store injected by application code never appears in the
	// configuration. Reading the configuration would call this application
	// unconfigured and refuse to start it.
	It("accepts a verifier whose API keys come from outside the configuration", func() {
		Expect(checkAuthConfiguration(moduleWith(nil), keysOnly)).To(Succeed())
	})

	Describe("an endpoint that insists on one kind of credential", func() {
		apiKeyOnly := &restkit.SecuritySpec{Schemes: []string{authkit.SchemeAPIKey}}

		// Requiring API keys while only JWTs are verifiable closes the endpoint
		// to everyone: no caller can ever present an accepted credential.
		It("is refused when nothing verifies that kind", func() {
			err := checkAuthConfiguration(moduleWith(apiKeyOnly), tokensOnly)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(authkit.SchemeAPIKey))
		})

		It("names what is available so the mismatch is visible", func() {
			err := checkAuthConfiguration(moduleWith(apiKeyOnly), tokensOnly)

			Expect(err.Error()).To(ContainSubstring(authkit.SchemeJWT))
		})

		It("is accepted once something verifies that kind", func() {
			Expect(checkAuthConfiguration(moduleWith(apiKeyOnly), keysOnly)).To(Succeed())
		})

		// A public endpoint is served without any credential, so naming a
		// scheme on it cannot lock anyone out.
		//
		// The set also holds an endpoint that needs a caller, otherwise the
		// check would stop before ever looking at the schemes.
		It("is ignored on a public endpoint", func() {
			public := &restkit.SecuritySpec{Public: true, Schemes: []string{authkit.SchemeAPIKey}}

			Expect(checkAuthConfiguration(moduleWith(public, nil), tokensOnly)).To(Succeed())
		})
	})

	It("accepts an endpoint that names no scheme, whatever can be verified", func() {
		wide := &restkit.SecuritySpec{Scopes: []string{"things:read"}}

		Expect(checkAuthConfiguration(moduleWith(wide), keysOnly)).To(Succeed())
	})
})
