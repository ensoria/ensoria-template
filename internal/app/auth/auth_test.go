package auth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the secret shipped with the template", func() {
	// The template ships a working secret so a checkout boots. Because it is
	// published, a deployment that keeps it can have its tokens forged by
	// anyone, so it must not survive past a developer's own machine.
	DescribeTable("is refused outside the environments it exists for",
		func(envVal string, allowed bool) {
			err := checkDevSecret(envVal, DevSecret)

			if allowed {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(envVal))
		},
		Entry("local", "local", true),
		Entry("test", "test", true),
		Entry("development", "development", false),
		Entry("staging", "staging", false),
		Entry("production", "production", false),
	)

	It("says how to fix it rather than only that it is wrong", func() {
		err := checkDevSecret("production", DevSecret)

		Expect(err.Error()).To(ContainSubstring("AUTH_SECRET"))
		Expect(err.Error()).To(ContainSubstring("jwks"))
	})

	It("leaves any other secret alone in every environment", func() {
		for _, envVal := range []string{"local", "test", "development", "staging", "production"} {
			Expect(checkDevSecret(envVal, "a-secret-of-our-own")).To(Succeed(), envVal)
		}
	})

	// An empty secret is the "nothing configured" case, which the startup check
	// in the HTTP pipeline reports; this guard must not claim it separately.
	It("says nothing about an unset secret", func() {
		Expect(checkDevSecret("production", "")).To(Succeed())
	})
})
