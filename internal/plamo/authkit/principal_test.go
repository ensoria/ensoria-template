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
	Describe("a scope policy attached to the caller", func() {
		// The policy is what lets an endpoint declare the permission it needs
		// and still be reachable by a scope that stands for it.
		It("lets a held scope satisfy a requirement it implies", func() {
			principal := (&authkit.Principal{Scopes: []string{"admin"}}).
				WithScopeExpander(implications(map[string][]string{
					"admin": {"orders:write"},
				}))

			Expect(principal.SatisfiesScopes([]string{"orders:write"})).To(BeTrue())
		})

		// Every scheme goes through the same attachment, so none of them may be
		// judged on a different set of scopes than the others.
		DescribeTable("applies to a caller of any scheme",
			func(scheme string) {
				principal := (&authkit.Principal{Subject: "usr_1", Scheme: scheme, Scopes: []string{"admin"}}).
					WithScopeExpander(implications(map[string][]string{
						"admin": {"orders:write"},
					}))

				Expect(principal.SatisfiesScopes([]string{"orders:write"})).To(BeTrue())
			},
			Entry("a token", authkit.SchemeJWT),
			Entry("an API key", authkit.SchemeAPIKey),
			Entry("a session cookie", authkit.SchemeSession),
		)

		It("still refuses a requirement nothing held implies", func() {
			principal := (&authkit.Principal{Scopes: []string{"admin"}}).
				WithScopeExpander(implications(map[string][]string{
					"admin": {"orders:write"},
				}))

			Expect(principal.SatisfiesScopes([]string{"users:write"})).To(BeFalse())
		})

		// Attaching in place would let one request's policy follow a value that
		// a key store hands to another request.
		It("is attached to a copy, leaving the caller it came from alone", func() {
			original := &authkit.Principal{Scopes: []string{"admin"}}

			attached := original.WithScopeExpander(implications(map[string][]string{
				"admin": {"orders:write"},
			}))

			Expect(attached).NotTo(BeIdenticalTo(original))
			Expect(attached.SatisfiesScopes([]string{"orders:write"})).To(BeTrue())
			Expect(original.SatisfiesScopes([]string{"orders:write"})).To(BeFalse())
		})

		// No policy is a real choice: it is what a deployment whose identity
		// provider already issues every scope asks for.
		It("changes nothing when no policy is attached", func() {
			principal := (&authkit.Principal{Scopes: []string{"admin"}}).WithScopeExpander(nil)

			Expect(principal.SatisfiesScopes([]string{"orders:write"})).To(BeFalse())
			Expect(principal.EffectiveScopes()).To(Equal([]string{"admin"}))
		})

		// ⚠ The credential's own scopes must stay as they arrived: a session is
		// created from this slice, and expanding it in place would freeze
		// today's policy into every session created from then on.
		It("leaves Scopes as the credential carried them", func() {
			principal := (&authkit.Principal{Scopes: []string{"admin"}}).
				WithScopeExpander(implications(map[string][]string{
					"admin": {"orders:write"},
				}))

			Expect(principal.Scopes).To(Equal([]string{"admin"}))
			Expect(principal.EffectiveScopes()).To(Equal([]string{"admin", "orders:write"}))
		})

		Describe("EffectiveScopes", func() {
			It("is empty for no caller", func() {
				var missing *authkit.Principal

				Expect(missing.EffectiveScopes()).To(BeEmpty())
			})

			It("hands back a copy when there is no policy", func() {
				principal := &authkit.Principal{Scopes: []string{"admin"}}

				principal.EffectiveScopes()[0] = "everything"

				Expect(principal.Scopes).To(Equal([]string{"admin"}))
			})
		})
	})

	Describe("SatisfiesScopes", func() {
		principal := &authkit.Principal{Scopes: []string{"users:read", "users:write"}}

		It("accepts a caller holding every required scope", func() {
			Expect(principal.SatisfiesScopes([]string{"users:read"})).To(BeTrue())
			Expect(principal.SatisfiesScopes([]string{"users:read", "users:write"})).To(BeTrue())
		})

		It("rejects a caller missing any one of them", func() {
			Expect(principal.SatisfiesScopes([]string{"users:read", "users:delete"})).To(BeFalse())
		})

		It("accepts when nothing is required", func() {
			Expect(principal.SatisfiesScopes(nil)).To(BeTrue())
		})

		It("rejects everything on a nil principal instead of panicking", func() {
			var missing *authkit.Principal

			Expect(missing.SatisfiesScopes(nil)).To(BeFalse())
			Expect(missing.SatisfiesScopes([]string{"users:read"})).To(BeFalse())
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
