package describe

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The failure being guarded against is a module gaining a dependency that
// stubs.go does not carry. It is not hypothetical: it happened, and nobody
// noticed until somebody ran the document generator, because this package had no
// test at all. fx builds lazily, so the hole only opens when a module reaches
// through it — which makes a test that resolves the whole graph the only thing
// that can find it early.
//
// Configuration is deliberately not initialized: building the graph does not need
// it, and keeping it that way matters, because config's once fires a single time
// per process and would tie the specs to each other's order.
var _ = Describe("resolveHTTPModules", func() {
	It("resolves the http_modules group with no real infrastructure", func() {
		modules, err := resolveHTTPModules()

		Expect(err).NotTo(HaveOccurred())
		// An empty group resolves just as cleanly as a full one, and would mean
		// every endpoint had silently dropped out of the generated document.
		Expect(modules).NotTo(BeEmpty())
		for _, module := range modules {
			Expect(module).NotTo(BeNil())
		}
	})
})
