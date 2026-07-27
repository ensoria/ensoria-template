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
})
