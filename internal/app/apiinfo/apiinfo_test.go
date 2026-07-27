package apiinfo_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/app/apiinfo"
)

var _ = Describe("Info", func() {
	// OpenAPI requires info.title and info.version, so the template must always
	// ship non-empty values even before a client customizes them.
	It("declares a non-empty title and version", func() {
		info := apiinfo.Info()

		Expect(info).NotTo(BeNil())
		Expect(info.Title).NotTo(BeEmpty())
		Expect(info.Version).NotTo(BeEmpty())
	})

	// A license without a name cannot be rendered, so the declaration must carry one
	// whenever a license is declared at all.
	It("names the license it declares", func() {
		license := apiinfo.Info().License

		Expect(license).NotTo(BeNil())
		Expect(license.Name).NotTo(BeEmpty())
	})
})
