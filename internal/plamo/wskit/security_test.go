package wskit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
)

var _ = Describe("SecuritySpec", func() {
	jwtCaller := func(scopes ...string) *authkit.Principal {
		return &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT, Scopes: scopes}
	}

	Describe("a channel with no declaration", func() {
		var security *wskit.SecuritySpec

		// A channel whose author never thought about access ends up closed:
		// it is an entry point from outside, and forgetting must not open it.
		It("refuses an anonymous caller as unauthenticated", func() {
			Expect(security.Decide(nil)).To(Equal(wskit.Unauthenticated))
		})

		It("admits any verified caller", func() {
			Expect(security.Decide(jwtCaller())).To(Equal(wskit.Allowed))
		})

		It("requires a caller", func() {
			Expect(security.RequiresCaller()).To(BeTrue())
		})
	})

	Describe("a public channel", func() {
		public := &wskit.SecuritySpec{Public: true}

		It("admits an anonymous caller", func() {
			Expect(public.Decide(nil)).To(Equal(wskit.Allowed))
		})

		It("admits a verified caller too", func() {
			Expect(public.Decide(jwtCaller())).To(Equal(wskit.Allowed))
		})

		It("requires no caller", func() {
			Expect(public.RequiresCaller()).To(BeFalse())
		})
	})

	Describe("required scopes", func() {
		needsRead := &wskit.SecuritySpec{Scopes: []string{"things:read"}}

		It("refuses an anonymous caller as unauthenticated", func() {
			Expect(needsRead.Decide(nil)).To(Equal(wskit.Unauthenticated))
		})

		It("refuses a caller without the scope as forbidden", func() {
			Expect(needsRead.Decide(jwtCaller())).To(Equal(wskit.Forbidden))
		})

		It("admits a caller with the scope", func() {
			Expect(needsRead.Decide(jwtCaller("things:read"))).To(Equal(wskit.Allowed))
		})

		It("requires every scope listed, not any of them", func() {
			both := &wskit.SecuritySpec{Scopes: []string{"things:read", "things:write"}}

			Expect(both.Decide(jwtCaller("things:read"))).To(Equal(wskit.Forbidden))
			Expect(both.Decide(jwtCaller("things:read", "things:write"))).To(Equal(wskit.Allowed))
		})

		// Judged through the caller, so the application's scope policy counts:
		// a caller holding only admin meets a channel asking for what admin
		// stands for. Forgetting the policy would refuse such a caller quietly.
		It("counts what the caller's scopes imply", func() {
			expander, err := authkit.NewScopeImplications(map[string][]string{"admin": {"things:read"}})
			Expect(err).NotTo(HaveOccurred())

			admin := jwtCaller("admin").WithScopeExpander(expander)

			Expect(needsRead.Decide(admin)).To(Equal(wskit.Allowed))
		})
	})

	Describe("accepted schemes", func() {
		keysOnly := &wskit.SecuritySpec{Schemes: []string{authkit.SchemeAPIKey}}

		It("refuses a caller who presented another kind of credential as forbidden", func() {
			Expect(keysOnly.Decide(jwtCaller())).To(Equal(wskit.Forbidden))
		})

		It("admits a caller who presented the accepted kind", func() {
			Expect(keysOnly.Decide(&authkit.Principal{Subject: "apikey", Scheme: authkit.SchemeAPIKey})).To(Equal(wskit.Allowed))
		})
	})

	Describe("Contradictory", func() {
		DescribeTable("is true only for Public stated with a condition",
			func(security *wskit.SecuritySpec, contradictory bool) {
				Expect(security.Contradictory()).To(Equal(contradictory))
			},
			Entry("nil", nil, false),
			Entry("public", &wskit.SecuritySpec{Public: true}, false),
			Entry("scopes only", &wskit.SecuritySpec{Scopes: []string{"things:read"}}, false),
			Entry("public with scopes", &wskit.SecuritySpec{Public: true, Scopes: []string{"things:read"}}, true),
			Entry("public with schemes", &wskit.SecuritySpec{Public: true, Schemes: []string{authkit.SchemeJWT}}, true),
		)
	})
})

var _ = Describe("MessageSecurity", func() {
	jwtCaller := func(scopes ...string) *authkit.Principal {
		return &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT, Scopes: scopes}
	}

	// ⚠ The opposite of a channel's nil, which requires a caller. A message's
	// declaration can only add to the channel's — a connection exists only once
	// the channel's conditions were met — so declaring nothing adds nothing.
	Describe("a message with no declaration", func() {
		var security *wskit.MessageSecurity

		It("adds nothing, admitting even an anonymous caller", func() {
			Expect(security.Decide(nil)).To(Equal(wskit.Allowed))
			Expect(security.Decide(jwtCaller())).To(Equal(wskit.Allowed))
		})

		It("requires no caller", func() {
			Expect(security.RequiresCaller()).To(BeFalse())
		})
	})

	// The way to say "signed in" on a channel that admits anonymous callers.
	Describe("an empty declaration", func() {
		signedIn := &wskit.MessageSecurity{}

		It("refuses an anonymous caller as unauthenticated", func() {
			Expect(signedIn.Decide(nil)).To(Equal(wskit.Unauthenticated))
		})

		It("admits any verified caller", func() {
			Expect(signedIn.Decide(jwtCaller())).To(Equal(wskit.Allowed))
		})

		It("requires a caller", func() {
			Expect(signedIn.RequiresCaller()).To(BeTrue())
		})
	})

	Describe("required scopes", func() {
		needsWrite := &wskit.MessageSecurity{Scopes: []string{"things:write"}}

		It("refuses an anonymous caller as unauthenticated", func() {
			Expect(needsWrite.Decide(nil)).To(Equal(wskit.Unauthenticated))
		})

		It("refuses a caller without the scope as forbidden", func() {
			Expect(needsWrite.Decide(jwtCaller("things:read"))).To(Equal(wskit.Forbidden))
		})

		It("admits a caller with the scope", func() {
			Expect(needsWrite.Decide(jwtCaller("things:write"))).To(Equal(wskit.Allowed))
		})

		It("requires every scope listed, not any of them", func() {
			both := &wskit.MessageSecurity{Scopes: []string{"things:read", "things:write"}}

			Expect(both.Decide(jwtCaller("things:write"))).To(Equal(wskit.Forbidden))
			Expect(both.Decide(jwtCaller("things:read", "things:write"))).To(Equal(wskit.Allowed))
		})

		It("counts what the caller's scopes imply", func() {
			expander, err := authkit.NewScopeImplications(map[string][]string{"admin": {"things:write"}})
			Expect(err).NotTo(HaveOccurred())

			admin := jwtCaller("admin").WithScopeExpander(expander)

			Expect(needsWrite.Decide(admin)).To(Equal(wskit.Allowed))
		})
	})
})
