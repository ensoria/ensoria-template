package initiator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
)

// mustClaimSet builds a claim set the spec knows to be valid.
func mustClaimSet(names ...string) *initiator.ClaimSet {
	GinkgoHelper()

	set, err := initiator.NewClaimSet(names)
	Expect(err).NotTo(HaveOccurred())
	return set
}

// atEntry is a request context the way the authentication middleware leaves it:
// marked as an entry point, carrying the caller when there is one.
func atEntry(caller *authkit.Principal, claims *initiator.ClaimSet) context.Context {
	ctx := initiator.WithEntry(context.Background(), claims)
	if caller != nil {
		ctx = authkit.WithPrincipal(ctx, caller)
	}
	return ctx
}

var _ = Describe("From", func() {
	// The table of TODO-auth-6.1 §1.4: what each origin of work reads as.
	Describe("at an entry point", func() {
		It("reads a caller signed in with a session as an application user", func() {
			from := initiator.From(atEntry(&authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeSession}, nil))

			Expect(from.Kind()).To(Equal(initiator.KindAppUser))
			Expect(from.ID()).To(Equal("user-1"))
		})

		// A token obtained with client credentials is a service's, and still
		// reads as an application user: the kind is a hint for audits and logs,
		// never an input to authorization.
		It("reads a caller with a token as an application user, whoever the token is for", func() {
			from := initiator.From(atEntry(&authkit.Principal{Subject: "service-a", Scheme: authkit.SchemeJWT}, nil))

			Expect(from.Kind()).To(Equal(initiator.KindAppUser))
			Expect(from.ID()).To(Equal("service-a"))
		})

		It("reads a caller with an API key as an API key", func() {
			from := initiator.From(atEntry(&authkit.Principal{Subject: "apikey", Scheme: authkit.SchemeAPIKey}, nil))

			Expect(from.Kind()).To(Equal(initiator.KindAPIKey))
			Expect(from.ID()).To(Equal("apikey"))
		})

		It("reads a request with no credential as unauthenticated", func() {
			from := initiator.From(atEntry(nil, nil))

			Expect(from.Kind()).To(Equal(initiator.KindUnauthenticated))
			Expect(from.ID()).To(BeEmpty())
		})

		It("carries only the claims the application carries, and only those the caller has", func() {
			caller := &authkit.Principal{
				Subject: "user-1",
				Scheme:  authkit.SchemeJWT,
				Claims:  authkit.Claims{"tenant": "t-1", "email": "x@example.com", "exp": 1700000000.0},
			}

			from := initiator.From(atEntry(caller, mustClaimSet("tenant", "department")))

			Expect(from.Claims()).To(Equal(authkit.Claims{"tenant": "t-1"}))
		})

		It("carries no claims when the application carries none", func() {
			caller := &authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeJWT, Claims: authkit.Claims{"tenant": "t-1"}}

			Expect(initiator.From(atEntry(caller, nil)).Claims()).To(BeNil())
		})

		// The value is derived from the caller when it is read, so a caller put
		// on the context later — a WebSocket connection rechecking its
		// credential — is the one read.
		It("reads the caller currently on the context", func() {
			ctx := atEntry(&authkit.Principal{Subject: "before", Scheme: authkit.SchemeSession}, nil)
			ctx = authkit.WithPrincipal(ctx, &authkit.Principal{Subject: "after", Scheme: authkit.SchemeSession})

			Expect(initiator.From(ctx).ID()).To(Equal("after"))
		})
	})

	It("reads work started by the scheduler as the system, with no ID", func() {
		from := initiator.From(initiator.WithSystem(context.Background()))

		Expect(from.Kind()).To(Equal(initiator.KindSystem))
		Expect(from.ID()).To(BeEmpty())
	})

	It("reads a context nobody marked as unknown", func() {
		from := initiator.From(context.Background())

		Expect(from.Kind()).To(Equal(initiator.KindUnknown))
		Expect(from.ID()).To(BeEmpty())
		Expect(from.Claims()).To(BeNil())
	})

	// Only the entry point can tell "anonymous" from "forgotten", but a caller
	// on the context leaves no doubt about who it is.
	It("reads a caller on a context without an entry mark, carrying no claims", func() {
		ctx := authkit.WithPrincipal(context.Background(), &authkit.Principal{
			Subject: "user-1", Scheme: authkit.SchemeJWT, Claims: authkit.Claims{"tenant": "t-1"},
		})

		from := initiator.From(ctx)

		Expect(from.Kind()).To(Equal(initiator.KindAppUser))
		Expect(from.ID()).To(Equal("user-1"))
		Expect(from.Claims()).To(BeNil())
	})

	It("reads a restored value as it was restored", func() {
		restored := initiator.FromHeaders(map[string]string{
			initiator.HeaderKind: string(initiator.KindAppUser),
			initiator.HeaderID:   "user-1",
		}, nil, "test")

		from := initiator.From(initiator.With(context.Background(), restored))

		Expect(from.Kind()).To(Equal(initiator.KindAppUser))
		Expect(from.ID()).To(Equal("user-1"))
	})

	Describe("when the context was marked more than once", func() {
		It("reads the mark put on last", func() {
			ctx := atEntry(&authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeSession}, nil)

			Expect(initiator.From(initiator.WithSystem(ctx)).Kind()).To(Equal(initiator.KindSystem))
		})

		It("derives from the caller again once an entry mark is put back on", func() {
			ctx := initiator.WithSystem(context.Background())
			ctx = authkit.WithPrincipal(initiator.WithEntry(ctx, nil), &authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeAPIKey})

			Expect(initiator.From(ctx).Kind()).To(Equal(initiator.KindAPIKey))
		})
	})

	It("hands out a copy, so changing what was read changes nothing", func() {
		caller := &authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeJWT, Claims: authkit.Claims{"tenant": "t-1"}}
		ctx := atEntry(caller, mustClaimSet("tenant"))

		initiator.From(ctx).Claims()["tenant"] = "changed"

		Expect(initiator.From(ctx).Claims()).To(Equal(authkit.Claims{"tenant": "t-1"}))
		Expect(caller.Claims).To(Equal(authkit.Claims{"tenant": "t-1"}))
	})

	It("reads claims with the typed accessors", func() {
		caller := &authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeJWT, Claims: authkit.Claims{"level": 3.0}}

		level, ok := initiator.From(atEntry(caller, mustClaimSet("level"))).Claims().Int("level")

		Expect(ok).To(BeTrue())
		Expect(level).To(BeEquivalentTo(3))
	})
})
