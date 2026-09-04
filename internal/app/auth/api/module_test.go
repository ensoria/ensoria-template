package api_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/app/auth/api"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

var _ = Describe("NewSessionModule", func() {
	It("serves both halves of a session's life on one path", func() {
		module := api.NewSessionModule(nil, nil)

		Expect(module.Path).To(Equal("/session"))
		Expect(module.Post).NotTo(BeNil())
		Expect(module.Delete).NotTo(BeNil())
	})

	// The startup check reads this to notice endpoints registered without a
	// store. It reads the declarations rather than the configuration, so the
	// module has to actually declare the scheme for that to work.
	It("declares the session scheme, which is what the startup check looks for", func() {
		modules := []*rest.Module{api.NewSessionModule(nil, nil)}

		Expect(restkit.DeclaredSchemes(modules)).To(ContainElement(authkit.SchemeSession))
	})

	// The document generator resolves every module with nothing behind it.
	// Failing here would break documentation for an application that is
	// perfectly well configured.
	It("builds without a store, because document generation has none", func() {
		Expect(func() { api.NewSessionModule(nil, nil) }).NotTo(Panic())
	})
})
