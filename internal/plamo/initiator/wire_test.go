package initiator_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
)

var _ = Describe("the wire form", func() {
	It("reserves the three header names", func() {
		Expect(initiator.ReservedHeaders()).To(ConsistOf(initiator.HeaderKind, initiator.HeaderID, initiator.HeaderClaims))
		Expect(initiator.IsReservedHeader("ce_authtype")).To(BeTrue())
		Expect(initiator.IsReservedHeader("trace_id")).To(BeFalse())
	})

	It("uses the CloudEvents authcontext attribute names", func() {
		Expect(initiator.HeaderKind).To(Equal("ce_authtype"))
		Expect(initiator.HeaderID).To(Equal("ce_authid"))
		Expect(initiator.HeaderClaims).To(Equal("ce_authclaims"))
	})

	Describe("Headers", func() {
		It("always writes the kind, even when it is unknown", func() {
			Expect(initiator.From(context.Background()).Headers()).To(Equal(map[string]string{
				initiator.HeaderKind: string(initiator.KindUnknown),
			}))
		})

		It("writes the ID and the claims only when there are some", func() {
			headers := initiator.From(atEntry(nil, nil)).Headers()

			Expect(headers).To(Equal(map[string]string{initiator.HeaderKind: string(initiator.KindUnauthenticated)}))
		})

		It("writes the claims as a JSON object in plain UTF-8", func() {
			caller := &authkit.Principal{
				Subject: "user-1",
				Scheme:  authkit.SchemeSession,
				Claims:  authkit.Claims{"display_name": "山田 <太郎>"},
			}

			headers := initiator.From(atEntry(caller, mustClaimSet("display_name"))).Headers()

			Expect(headers[initiator.HeaderKind]).To(Equal(string(initiator.KindAppUser)))
			Expect(headers[initiator.HeaderID]).To(Equal("user-1"))
			Expect(headers[initiator.HeaderClaims]).To(Equal(`{"display_name":"山田 <太郎>"}`))
		})

		// The value is informational: a claim that cannot be written must not
		// keep the message from being sent.
		It("leaves out claims that cannot be written as JSON, and says so", func() {
			caller := &authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeAPIKey, Claims: authkit.Claims{"bad": make(chan int)}}
			from := initiator.From(atEntry(caller, mustClaimSet("bad")))

			var headers map[string]string
			records := captureLogRecords(func() { headers = from.Headers() })

			Expect(headers).To(Equal(map[string]string{
				initiator.HeaderKind: string(initiator.KindAPIKey),
				initiator.HeaderID:   "user-1",
			}))
			Expect(recordsOfType(records, initiator.LogTypeClaimsUnencodable)).To(HaveLen(1))
		})
	})

	Describe("FromHeaders", func() {
		It("restores what Headers wrote", func() {
			set := mustClaimSet("tenant", "level", "roles")
			caller := &authkit.Principal{
				Subject: "user-1",
				Scheme:  authkit.SchemeJWT,
				Claims:  authkit.Claims{"tenant": "t-1", "level": 3, "roles": []string{"editor"}},
			}

			restored := initiator.FromHeaders(initiator.From(atEntry(caller, set)).Headers(), set, "hello_world")

			Expect(restored.Kind()).To(Equal(initiator.KindAppUser))
			Expect(restored.ID()).To(Equal("user-1"))
			tenant, _ := restored.Claims().String("tenant")
			Expect(tenant).To(Equal("t-1"))
			level, ok := restored.Claims().Int("level")
			Expect(ok).To(BeTrue())
			Expect(level).To(BeEquivalentTo(3))
			roles, _ := restored.Claims().StringSlice("roles")
			Expect(roles).To(Equal([]string{"editor"}))
		})

		// Nothing at all means a producer that does not follow the convention.
		// That is not malformed, just unknown.
		It("reads no headers as unknown, quietly", func() {
			var restored *initiator.Initiator
			records := captureLogRecords(func() {
				restored = initiator.FromHeaders(map[string]string{"routing_key": "q"}, nil, "hello_world")
			})

			Expect(restored.Kind()).To(Equal(initiator.KindUnknown))
			Expect(records).To(BeEmpty())
		})

		// CloudEvents lets implementations add values of their own.
		It("takes an unfamiliar kind as it is", func() {
			var restored *initiator.Initiator
			records := captureLogRecords(func() {
				restored = initiator.FromHeaders(map[string]string{initiator.HeaderKind: "robot", initiator.HeaderID: "r2"}, nil, "hello_world")
			})

			Expect(restored.Kind()).To(Equal(initiator.Kind("robot")))
			Expect(restored.ID()).To(Equal("r2"))
			Expect(records).To(BeEmpty())
		})

		// A malformed value is never a reason to refuse the message: this is
		// context, not a gate. It is restored as unknown, with a warning.
		DescribeTable("reads a malformed value as unknown and warns",
			func(headers map[string]string) {
				var restored *initiator.Initiator
				records := captureLogRecords(func() {
					restored = initiator.FromHeaders(headers, mustClaimSet("tenant"), "hello_world")
				})

				Expect(restored.Kind()).To(Equal(initiator.KindUnknown))
				Expect(restored.ID()).To(BeEmpty())
				Expect(restored.Claims()).To(BeNil())
				warnings := recordsOfType(records, initiator.LogTypeHeaderMalformed)
				Expect(warnings).To(HaveLen(1))
				Expect(warnings[0]).To(HaveKeyWithValue("source", "hello_world"))
				Expect(warnings[0]).To(HaveKey("reason"))
			},
			Entry("an ID without a kind", map[string]string{initiator.HeaderID: "user-1"}),
			Entry("claims without a kind", map[string]string{initiator.HeaderClaims: `{"tenant":"t-1"}`}),
			Entry("an empty kind with an ID", map[string]string{initiator.HeaderKind: "", initiator.HeaderID: "user-1"}),
			Entry("claims that are not JSON", map[string]string{initiator.HeaderKind: "app_user", initiator.HeaderClaims: `{tenant`}),
			Entry("claims that are a JSON array", map[string]string{initiator.HeaderKind: "app_user", initiator.HeaderClaims: `["tenant"]`}),
			Entry("claims that are JSON null", map[string]string{initiator.HeaderKind: "app_user", initiator.HeaderClaims: `null`}),
			Entry("claims carrying scope", map[string]string{initiator.HeaderKind: "app_user", initiator.HeaderClaims: `{"tenant":"t-1","scope":"admin"}`}),
			Entry("claims carrying a registered claim", map[string]string{initiator.HeaderKind: "app_user", initiator.HeaderClaims: `{"exp":1700000000}`}),
		)

		It("does not put the malformed values in the warning", func() {
			records := captureLogRecords(func() {
				initiator.FromHeaders(map[string]string{initiator.HeaderID: "secret-user-id"}, nil, "hello_world")
			})

			raw, err := json.Marshal(records)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).NotTo(ContainSubstring("secret-user-id"))
		})
	})
})
