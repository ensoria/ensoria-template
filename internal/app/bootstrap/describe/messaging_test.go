package describe

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The messaging side shares stubs.go with the HTTP side but reaches a different
// part of the graph, so it can break while the HTTP side stays green — and it
// did the opposite once already, staying green while apidoc-describe was broken.
var _ = Describe("resolveMessagingModules", func() {
	It("resolves the three declaration groups with no real infrastructure", func() {
		modules, err := resolveMessagingModules()

		Expect(err).NotTo(HaveOccurred())
		Expect(modules).NotTo(BeNil())

		// As with the HTTP group, resolving to nothing would be a clean success
		// that produced an empty document.
		declared := len(modules.Subscriptions) + len(modules.Publications) + len(modules.Channels)
		Expect(declared).To(BeNumerically(">", 0))
	})
})
