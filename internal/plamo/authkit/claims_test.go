package authkit_test

import (
	"context"
	"encoding/json"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

var _ = Describe("Claims", func() {
	Describe("String", func() {
		It("returns a string claim", func() {
			org, ok := authkit.Claims{"org": "acme"}.String("org")

			Expect(ok).To(BeTrue())
			Expect(org).To(Equal("acme"))
		})

		// A KeyStore builds its caller in Go, where a claim may well have a type
		// of its own. Refusing it would deny a caller for the wrong reason.
		It("returns a value whose type is defined on a string", func() {
			type tenant string

			value, ok := authkit.Claims{"tenant": tenant("acme")}.String("tenant")

			Expect(ok).To(BeTrue())
			Expect(value).To(Equal("acme"))
		})

		DescribeTable("reports anything else as no string claim",
			func(value any) {
				_, ok := authkit.Claims{"claim": value}.String("claim")

				Expect(ok).To(BeFalse())
			},
			// A number written as JSON is a number, not the text of one.
			Entry("a JSON number", json.Number("3")),
			Entry("a number", float64(3)),
			Entry("a boolean", true),
			Entry("a list", []any{"acme"}),
			Entry("nothing", nil),
		)

		It("reports a claim that is not there", func() {
			_, ok := authkit.Claims{}.String("org")

			Expect(ok).To(BeFalse())
		})

		// A caller whose credential carried nothing else has no claims at all.
		It("reads from a caller with no claims", func() {
			var claims authkit.Claims

			_, ok := claims.String("org")

			Expect(ok).To(BeFalse())
		})
	})

	Describe("Bool", func() {
		It("returns a boolean claim", func() {
			verified, ok := authkit.Claims{"email_verified": true}.Bool("email_verified")

			Expect(ok).To(BeTrue())
			Expect(verified).To(BeTrue())
		})

		// Some issuers write this claim as a string. That is a different claim,
		// and reading it as true here would be this package deciding that "TRUE",
		// "1" and "yes" mean the same thing — a guess the issuer never made.
		DescribeTable("reports anything else as no boolean claim",
			func(value any) {
				_, ok := authkit.Claims{"claim": value}.Bool("claim")

				Expect(ok).To(BeFalse())
			},
			Entry("the string true", "true"),
			Entry("the number 1", float64(1)),
			Entry("nothing", nil),
		)
	})

	Describe("Number", func() {
		DescribeTable("returns a numeric claim however it was written",
			func(value any, want float64) {
				got, ok := authkit.Claims{"level": value}.Number("level")

				Expect(ok).To(BeTrue())
				Expect(got).To(Equal(want))
			},
			// What a verified token and a restored session produce.
			Entry("a float64", float64(3), float64(3)),
			// What --claim-json produces, and what a decoder using json.Number does.
			Entry("a JSON number", json.Number("3"), float64(3)),
			Entry("a fraction", 2.5, 2.5),
			// What a KeyStore building a caller in Go produces.
			Entry("an int", 3, float64(3)),
			Entry("an int64", int64(3), float64(3)),
			Entry("a uint", uint(3), float64(3)),
		)

		DescribeTable("reports anything else as no numeric claim",
			func(value any) {
				_, ok := authkit.Claims{"level": value}.Number("level")

				Expect(ok).To(BeFalse())
			},
			Entry("a number written as a string", "3"),
			Entry("a boolean", true),
			Entry("nothing", nil),
		)
	})

	Describe("Int", func() {
		DescribeTable("returns a whole-number claim however it was written",
			func(value any, want int64) {
				got, ok := authkit.Claims{"level": value}.Int("level")

				Expect(ok).To(BeTrue())
				Expect(got).To(Equal(want))
			},
			Entry("a float64", float64(3), int64(3)),
			Entry("a JSON number", json.Number("3"), int64(3)),
			Entry("an int", 3, int64(3)),
			Entry("a negative int", -3, int64(-3)),
			Entry("a uint64", uint64(3), int64(3)),
			// A JSON number keeps every digit it was written with, so a token
			// minted with encli auth token --claim-json reads back exactly.
			Entry("a JSON integer beyond a float64's reach",
				json.Number("9007199254740993"), int64(9007199254740993)),
		)

		// Truncating is the mistake these accessors exist to prevent: a rule
		// reading 2.5 as 2 is a rule nobody wrote.
		It("refuses a number with a fractional part", func() {
			_, ok := authkit.Claims{"level": 2.5}.Int("level")

			Expect(ok).To(BeFalse())
		})

		// The digits are already gone by the time this is read — 2^53+1 and 2^53
		// are the same float64 — so there is no answer to give.
		It("refuses a float64 too large to hold a whole number exactly", func() {
			_, ok := authkit.Claims{"id": math.Pow(2, 53) + 2}.Int("id")

			Expect(ok).To(BeFalse())
		})

		It("refuses a uint64 larger than an int64 can hold", func() {
			_, ok := authkit.Claims{"id": uint64(math.MaxUint64)}.Int("id")

			Expect(ok).To(BeFalse())
		})

		DescribeTable("reports anything else as no whole-number claim",
			func(value any) {
				_, ok := authkit.Claims{"level": value}.Int("level")

				Expect(ok).To(BeFalse())
			},
			Entry("a number written as a string", "3"),
			Entry("a boolean", true),
			Entry("nothing", nil),
		)
	})

	Describe("StringSlice", func() {
		DescribeTable("returns a list of strings however it was written",
			func(value any) {
				roles, ok := authkit.Claims{"roles": value}.StringSlice("roles")

				Expect(ok).To(BeTrue())
				Expect(roles).To(Equal([]string{"editor", "viewer"}))
			},
			// What a verified token and a restored session produce.
			Entry("a list of any", []any{"editor", "viewer"}),
			// What a KeyStore building a caller in Go produces.
			Entry("a list of strings", []string{"editor", "viewer"}),
		)

		It("returns an empty list as one", func() {
			roles, ok := authkit.Claims{"roles": []any{}}.StringSlice("roles")

			Expect(ok).To(BeTrue())
			Expect(roles).To(BeEmpty())
		})

		// Returning the strings that happened to be in it would judge a caller on
		// part of a claim.
		It("refuses a list holding anything but strings", func() {
			_, ok := authkit.Claims{"roles": []any{"editor", 2}}.StringSlice("roles")

			Expect(ok).To(BeFalse())
		})

		// A single value is not a list of one: whether "editor viewer" would be
		// one role or two has no answer this package may invent.
		It("refuses a single string", func() {
			_, ok := authkit.Claims{"roles": "editor"}.StringSlice("roles")

			Expect(ok).To(BeFalse())
		})

		// The claim map outlives the request that read it — a KeyStore may keep
		// the caller, and a session record is read again on the next request.
		It("returns a slice the caller can change without changing the claim", func() {
			claims := authkit.Claims{"roles": []string{"editor"}}

			roles, ok := claims.StringSlice("roles")
			Expect(ok).To(BeTrue())
			roles[0] = "admin"

			again, ok := claims.StringSlice("roles")
			Expect(ok).To(BeTrue())
			Expect(again).To(Equal([]string{"editor"}))
		})
	})

	// The whole point of the accessors: the same claim reads the same way
	// whichever credential the caller presented. A token and a session go through
	// JSON and arrive in JSON's types; a caller a KeyStore built never does.
	//
	// ⚠ The sources are a table so that another one can be added to it. Carrying
	// a caller over a message broker (TODO-auth-6) is the next one.
	Describe("the answers are the same whatever the caller presented", func() {
		for _, source := range principalSources() {
			Describe("a caller from "+source.name, func() {
				var claims authkit.Claims

				BeforeEach(func() {
					claims = source.build().Claims
				})

				It("reads a string claim", func() {
					org, ok := claims.String("org")

					Expect(ok).To(BeTrue())
					Expect(org).To(Equal("acme"))
				})

				It("reads a whole number claim", func() {
					level, ok := claims.Int("level")

					Expect(ok).To(BeTrue())
					Expect(level).To(BeEquivalentTo(3))
				})

				It("reads the same claim as a number", func() {
					level, ok := claims.Number("level")

					Expect(ok).To(BeTrue())
					Expect(level).To(Equal(float64(3)))
				})

				It("reads a fractional claim as a number and not as a whole one", func() {
					ratio, ok := claims.Number("ratio")
					Expect(ok).To(BeTrue())
					Expect(ratio).To(Equal(2.5))

					_, ok = claims.Int("ratio")
					Expect(ok).To(BeFalse())
				})

				It("reads a boolean claim", func() {
					active, ok := claims.Bool("active")

					Expect(ok).To(BeTrue())
					Expect(active).To(BeTrue())
				})

				It("reads a list of strings", func() {
					roles, ok := claims.StringSlice("roles")

					Expect(ok).To(BeTrue())
					Expect(roles).To(Equal([]string{"editor", "viewer"}))
				})

				// The same refusals, too: a source that answered here would be a
				// source rules behave differently on.
				It("refuses to read a number as a string", func() {
					_, ok := claims.String("level")

					Expect(ok).To(BeFalse())
				})

				It("reports a claim the credential did not carry", func() {
					_, ok := claims.String("department")

					Expect(ok).To(BeFalse())
				})
			})
		}
	})
})

// principalSource is one way a verified caller reaches application code.
type principalSource struct {
	name  string
	build func() *authkit.Principal
}

// principalSources lists them. Each builds a caller carrying the same claims —
// the same values, written the way that source would write them.
func principalSources() []principalSource {
	return []principalSource{
		{name: "a verified token", build: principalFromToken},
		{name: "a restored session", build: principalFromSession},
		{name: "an API key store", build: principalFromKeyStore},
	}
}

// principalFromToken verifies a signed token, so the claims arrive the way the
// JWT parser leaves them.
func principalFromToken() *authkit.Principal {
	GinkgoHelper()

	token := claims()
	token["org"] = "acme"
	token["level"] = 3
	token["ratio"] = 2.5
	token["active"] = true
	token["roles"] = []string{"editor", "viewer"}

	verifier, err := authkit.NewVerifier(hs256Config, nil, nil)
	Expect(err).NotTo(HaveOccurred())

	result, err := verifier.Verify(requestWith(bearer(signHS(token))))
	Expect(err).NotTo(HaveOccurred())
	Expect(result.Principal).NotTo(BeNil())
	return result.Principal
}

// principalFromSession takes the caller from a token, stores the session the way
// the token-exchange endpoint does, and reads it back — through the store's real
// codec, which is what puts the claims into JSON's types a second time.
func principalFromSession() *authkit.Principal {
	GinkgoHelper()

	cfg, err := sessionkit.NewConfig(sessionConfig())
	Expect(err).NotTo(HaveOccurred())
	store, err := sessionkit.NewStore(cachememory.New("test-sessions"), cfg)
	Expect(err).NotTo(HaveOccurred())

	created, err := store.Create(context.Background(), authkit.SnapshotOf(principalFromToken()), false)
	Expect(err).NotTo(HaveOccurred())

	restored, err := store.Lookup(context.Background(), created.ID)
	Expect(err).NotTo(HaveOccurred())
	return authkit.PrincipalOf(restored.Snapshot)
}

// principalFromKeyStore builds the caller in Go and never serializes it, which
// is what a KeyStore backed by a database does.
func principalFromKeyStore() *authkit.Principal {
	GinkgoHelper()

	keys := authkit.KeyStoreFunc(func(context.Context, string) (*authkit.Principal, error) {
		return &authkit.Principal{
			Subject: "payment-provider",
			Scheme:  authkit.SchemeAPIKey,
			Claims: authkit.Claims{
				"org":    "acme",
				"level":  3,
				"ratio":  2.5,
				"active": true,
				"roles":  []string{"editor", "viewer"},
			},
		}, nil
	})

	verifier, err := authkit.NewVerifier(hs256Config, keys, nil)
	Expect(err).NotTo(HaveOccurred())

	result, err := verifier.Verify(requestWith(map[string]string{appconfig.DefaultAPIKeyHeader: "a-key"}))
	Expect(err).NotTo(HaveOccurred())
	Expect(result.Principal).NotTo(BeNil())
	return result.Principal
}
