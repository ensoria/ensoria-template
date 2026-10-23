package initiator_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
)

var _ = Describe("NewClaimSet", func() {
	It("accepts an empty list, which carries nothing", func() {
		set, err := initiator.NewClaimSet(nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(set.Names()).To(BeEmpty())
	})

	It("lists the names in a stable order, each once", func() {
		set := mustClaimSet("tenant", "department", "tenant")

		Expect(set.Names()).To(Equal([]string{"department", "tenant"}))
	})

	It("hands out a copy of the names", func() {
		set := mustClaimSet("tenant")

		set.Names()[0] = "changed"

		Expect(set.Names()).To(Equal([]string{"tenant"}))
	})

	It("treats a nil set as the empty one", func() {
		var set *initiator.ClaimSet

		Expect(set.Names()).To(BeEmpty())
	})

	// scope would let unverified data authorize on the far side of a broker,
	// and the registered claims describe the original token, which means
	// nothing once carried elsewhere.
	DescribeTable("refuses a claim that must not be carried",
		func(name string) {
			_, err := initiator.NewClaimSet([]string{"tenant", name})

			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("%q", name))))
		},
		Entry("scope", "scope"),
		Entry("iss", "iss"),
		Entry("sub", "sub"),
		Entry("aud", "aud"),
		Entry("exp", "exp"),
		Entry("nbf", "nbf"),
		Entry("iat", "iat"),
		Entry("jti", "jti"),
	)

	It("refuses an empty name", func() {
		_, err := initiator.NewClaimSet([]string{""})

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("claims received from elsewhere", func() {
	// headersWith is a well-formed wire form carrying the given claims JSON.
	headersWith := func(claimsJSON string) map[string]string {
		return map[string]string{
			initiator.HeaderKind:   string(initiator.KindAppUser),
			initiator.HeaderID:     "user-1",
			initiator.HeaderClaims: claimsJSON,
		}
	}

	It("keeps the claims the application carries and drops the rest", func() {
		from := initiator.FromHeaders(headersWith(`{"tenant":"t-1","email":"x@example.com"}`), mustClaimSet("tenant"), "hello_world")

		Expect(from.Claims()).To(Equal(authkit.Claims{"tenant": "t-1"}))
	})

	It("keeps the kind and ID when it drops a claim", func() {
		from := initiator.FromHeaders(headersWith(`{"email":"x@example.com"}`), nil, "hello_world")

		Expect(from.Kind()).To(Equal(initiator.KindAppUser))
		Expect(from.ID()).To(Equal("user-1"))
		Expect(from.Claims()).To(BeNil())
	})

	Describe("reporting a dropped claim", func() {
		It("names the claim and where it arrived, never its value", func() {
			set := mustClaimSet("tenant")

			records := captureLogRecords(func() {
				initiator.FromHeaders(headersWith(`{"email":"x@example.com"}`), set, "hello_world")
			})

			dropped := recordsOfType(records, initiator.LogTypeClaimDropped)
			Expect(dropped).To(HaveLen(1))
			Expect(dropped[0]).To(HaveKeyWithValue("claim", "email"))
			Expect(dropped[0]).To(HaveKeyWithValue("source", "hello_world"))
			Expect(fmt.Sprint(dropped[0])).NotTo(ContainSubstring("x@example.com"))
		})

		It("reports each name once", func() {
			set := mustClaimSet()

			records := captureLogRecords(func() {
				initiator.FromHeaders(headersWith(`{"email":"a@example.com"}`), set, "hello_world")
				initiator.FromHeaders(headersWith(`{"email":"b@example.com"}`), set, "user.created")
				initiator.FromHeaders(headersWith(`{"phone":"000"}`), set, "hello_world")
			})

			dropped := recordsOfType(records, initiator.LogTypeClaimDropped)
			Expect(dropped).To(HaveLen(2))
			Expect(dropped[0]).To(HaveKeyWithValue("claim", "email"))
			Expect(dropped[1]).To(HaveKeyWithValue("claim", "phone"))
		})

		// The names come from other producers, which this application does not
		// control: neither the memory nor the log may grow without bound.
		It("stops reporting new names after a limit, and says so once", func() {
			set := mustClaimSet()

			records := captureLogRecords(func() {
				for i := 0; i < initiator.MaxReportedDroppedClaims+5; i++ {
					initiator.FromHeaders(headersWith(fmt.Sprintf(`{"claim_%d":true}`, i)), set, "hello_world")
				}
			})

			dropped := recordsOfType(records, initiator.LogTypeClaimDropped)
			Expect(dropped).To(HaveLen(initiator.MaxReportedDroppedClaims + 1))
			Expect(dropped[initiator.MaxReportedDroppedClaims]).NotTo(HaveKey("claim"))
		})

		It("keeps its memory per claim set", func() {
			records := captureLogRecords(func() {
				initiator.FromHeaders(headersWith(`{"email":"x@example.com"}`), mustClaimSet(), "hello_world")
				initiator.FromHeaders(headersWith(`{"email":"x@example.com"}`), mustClaimSet(), "hello_world")
			})

			Expect(recordsOfType(records, initiator.LogTypeClaimDropped)).To(HaveLen(2))
		})
	})
})
