package authkit_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

var _ = Describe("Credential", func() {
	// A token cannot be ended early by anyone, so its expiry is all there is
	// to know about it later.
	It("keeps a token's expiry", func() {
		expires := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

		credential := authkit.TokenCredential(expires)

		Expect(credential.Scheme()).To(Equal(authkit.SchemeJWT))
		at, ok := credential.Expires()
		Expect(ok).To(BeTrue())
		Expect(at).To(Equal(expires))
	})

	// A key or a session can be ended by its store at any moment, so it has
	// no expiry of its own to read: it has to be asked about.
	DescribeTable("has no expiry of its own for a credential its store can end",
		func(credential *authkit.Credential, scheme string) {
			Expect(credential.Scheme()).To(Equal(scheme))
			_, ok := credential.Expires()
			Expect(ok).To(BeFalse())
		},
		Entry("an API key", authkit.KeyCredential("a-key"), authkit.SchemeAPIKey),
		Entry("a session", authkit.SessionCredential("a-session-id"), authkit.SchemeSession),
	)

	It("reads as no credential when there is none", func() {
		var credential *authkit.Credential

		Expect(credential.Scheme()).To(BeEmpty())
		_, ok := credential.Expires()
		Expect(ok).To(BeFalse())
	})

	Describe("on a context", func() {
		It("is found where it was put", func() {
			credential := authkit.KeyCredential("a-key")

			found, ok := authkit.CredentialFrom(authkit.WithCredential(context.Background(), credential))

			Expect(ok).To(BeTrue())
			Expect(found).To(BeIdenticalTo(credential))
		})

		It("is absent from a context that carries none", func() {
			_, ok := authkit.CredentialFrom(context.Background())

			Expect(ok).To(BeFalse())
		})
	})
})
