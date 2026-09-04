package middleware_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/middleware"
)

var _ = Describe("ParseOrigins", func() {
	It("reads a comma-separated list, ignoring the spacing", func() {
		origins := middleware.ParseOrigins("https://app.example.test,  https://admin.example.test ")

		Expect(origins.Named()).To(Equal([]string{
			"https://app.example.test", "https://admin.example.test",
		}))
		Expect(origins.Configured()).To(BeTrue())
		Expect(origins.Wildcard()).To(BeFalse())
	})

	// The same-origin deployment. It claims no other origin, which is not the
	// same as claiming all of them.
	It("reads nothing at all as claiming no origin", func() {
		origins := middleware.ParseOrigins("")

		Expect(origins.Configured()).To(BeFalse())
		Expect(origins.Wildcard()).To(BeFalse())
		Expect(origins.Named()).To(BeEmpty())
	})

	It("reads the wildcard as claiming every origin, and names none", func() {
		origins := middleware.ParseOrigins("*")

		Expect(origins.Wildcard()).To(BeTrue())
		Expect(origins.Configured()).To(BeTrue())
		// The cross-origin check is given Named(): "every site" is not an
		// answer to the question it asks.
		Expect(origins.Named()).To(BeEmpty())
	})

	Describe("AllowedValue", func() {
		origins := middleware.ParseOrigins("https://app.example.test,https://admin.example.test")

		// ⚠ The defect this pins. Access-Control-Allow-Origin takes one origin
		// or the wildcard; answering with the configured list produces a header
		// every browser rejects, while looking correct in a server-side test.
		It("answers with the one origin that matched, never the list", func() {
			Expect(origins.AllowedValue("https://admin.example.test")).
				To(Equal("https://admin.example.test"))
		})

		It("answers with nothing for an origin nobody claimed", func() {
			Expect(origins.AllowedValue("https://evil.example")).To(BeEmpty())
		})

		// A caller that is not a browser sends no Origin. There is nothing to
		// tell it, and nothing it would do with the answer.
		It("answers with nothing when there is no origin", func() {
			Expect(origins.AllowedValue("")).To(BeEmpty())
		})

		It("matches exactly, so a different scheme or port is a different origin", func() {
			Expect(origins.AllowedValue("http://app.example.test")).To(BeEmpty())
			Expect(origins.AllowedValue("https://app.example.test:8443")).To(BeEmpty())
		})

		It("answers with the wildcard itself when every origin is claimed", func() {
			Expect(middleware.ParseOrigins("*").AllowedValue("https://anywhere.example")).
				To(Equal("*"))
		})
	})

	Describe("VariesByOrigin", func() {
		// What a cache has to be told: the answer given to one origin must not
		// be replayed to another.
		It("is true when the answer depends on who asked", func() {
			Expect(middleware.ParseOrigins("https://app.example.test").VariesByOrigin()).To(BeTrue())
		})

		It("is false for the wildcard, which answers everyone the same", func() {
			Expect(middleware.ParseOrigins("*").VariesByOrigin()).To(BeFalse())
		})

		It("is false when no origin is claimed", func() {
			Expect(middleware.ParseOrigins("").VariesByOrigin()).To(BeFalse())
		})
	})
})
