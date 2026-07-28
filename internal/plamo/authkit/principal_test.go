package authkit_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

var _ = Describe("Principal", func() {
	Describe("carrying it on a context", func() {
		It("returns what was put on the context", func() {
			principal := &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT}

			ctx := authkit.WithPrincipal(context.Background(), principal)

			got, ok := authkit.PrincipalFrom(ctx)
			Expect(ok).To(BeTrue())
			Expect(got).To(Equal(principal))
		})

		It("reports no principal on a bare context", func() {
			got, ok := authkit.PrincipalFrom(context.Background())

			Expect(ok).To(BeFalse())
			Expect(got).To(BeNil())
		})

		It("reports no principal when a nil one was put on the context", func() {
			ctx := authkit.WithPrincipal(context.Background(), nil)

			_, ok := authkit.PrincipalFrom(ctx)
			Expect(ok).To(BeFalse())
		})
	})

	// Scopes are checked with AND semantics, matching how OpenAPI reads
	// `security: [{scheme: [a, b]}]`: the caller needs every listed scope.
	Describe("HasScopes", func() {
		principal := &authkit.Principal{Scopes: []string{"users:read", "users:write"}}

		It("accepts a caller holding every required scope", func() {
			Expect(principal.HasScopes([]string{"users:read"})).To(BeTrue())
			Expect(principal.HasScopes([]string{"users:read", "users:write"})).To(BeTrue())
		})

		It("rejects a caller missing any one of them", func() {
			Expect(principal.HasScopes([]string{"users:read", "users:delete"})).To(BeFalse())
		})

		It("accepts when nothing is required", func() {
			Expect(principal.HasScopes(nil)).To(BeTrue())
		})

		It("rejects everything on a nil principal instead of panicking", func() {
			var missing *authkit.Principal

			Expect(missing.HasScopes(nil)).To(BeFalse())
			Expect(missing.HasScopes([]string{"users:read"})).To(BeFalse())
		})
	})

	Describe("HasScheme", func() {
		principal := &authkit.Principal{Scheme: authkit.SchemeAPIKey}

		It("accepts the scheme the caller authenticated with", func() {
			Expect(principal.HasScheme([]string{authkit.SchemeAPIKey})).To(BeTrue())
			Expect(principal.HasScheme([]string{authkit.SchemeJWT, authkit.SchemeAPIKey})).To(BeTrue())
		})

		It("rejects a scheme the caller did not use", func() {
			Expect(principal.HasScheme([]string{authkit.SchemeJWT})).To(BeFalse())
		})

		It("accepts any scheme when the endpoint names none", func() {
			Expect(principal.HasScheme(nil)).To(BeTrue())
		})
	})
})
