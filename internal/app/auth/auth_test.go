package auth

import (
	"github.com/ensoria/config/pkg/appconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// withSecret and withAPIKey build the configuration under test.
func withSecret(secret string) *appconfig.Auth { return &appconfig.Auth{Secret: secret} }

func withAPIKey(keys ...string) *appconfig.Auth { return &appconfig.Auth{APIKeys: keys} }

var _ = Describe("the secret shipped with the template", func() {
	// The template ships a working secret so a checkout boots. Because it is
	// published, a deployment that keeps it can have its tokens forged by
	// anyone, so it must not survive past a developer's own machine.
	DescribeTable("is refused outside the environments it exists for",
		func(envVal string, allowed bool) {
			err := checkDevCredentials(envVal, withSecret(DevSecret))

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
		err := checkDevCredentials("production", withSecret(DevSecret))

		Expect(err.Error()).To(ContainSubstring("AUTH_SECRET"))
		Expect(err.Error()).To(ContainSubstring("jwks"))
	})

	It("leaves any other secret alone in every environment", func() {
		for _, envVal := range []string{"local", "test", "development", "staging", "production"} {
			Expect(checkDevCredentials(envVal, withSecret("a-secret-of-our-own"))).To(Succeed(), envVal)
		}
	})

	// An empty secret is the "nothing configured" case, which the startup check
	// in the HTTP pipeline reports; this guard must not claim it separately.
	It("says nothing about an unset secret", func() {
		Expect(checkDevCredentials("production", withSecret(""))).To(Succeed())
	})
})

var _ = Describe("the API key shipped with the template", func() {
	It("is refused outside local and test", func() {
		err := checkDevCredentials("production", withAPIKey(DevAPIKey))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("AUTH_API_KEYS"))
	})

	// The key is refused even when it is one of several, because the published
	// key alone is enough to reach every endpoint that accepts an API key.
	It("is refused even alongside keys of our own", func() {
		err := checkDevCredentials("production", withAPIKey("a-key-of-our-own", DevAPIKey))

		Expect(err).To(HaveOccurred())
	})

	It("is allowed on a developer machine", func() {
		Expect(checkDevCredentials("local", withAPIKey(DevAPIKey))).To(Succeed())
	})

	It("leaves keys of our own alone", func() {
		Expect(checkDevCredentials("production", withAPIKey("a-key-of-our-own"))).To(Succeed())
	})
})
