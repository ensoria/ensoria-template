package initiator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
)

var _ = Describe("Propagator", func() {
	It("writes who started the work into the metadata, in the wire form", func() {
		ctx := atEntry(&authkit.Principal{Subject: "user-1", Scheme: authkit.SchemeSession}, nil)
		metadata := map[string]string{"trace_id": "t-1"}

		initiator.NewPropagator(nil, "worker job").Inject(ctx, metadata)

		Expect(metadata).To(Equal(map[string]string{
			"trace_id":           "t-1",
			initiator.HeaderKind: string(initiator.KindAppUser),
			initiator.HeaderID:   "user-1",
		}))
	})

	It("puts it back on the context it is handed", func() {
		propagator := initiator.NewPropagator(mustClaimSet("tenant"), "worker job")
		metadata := map[string]string{}
		propagator.Inject(atEntry(&authkit.Principal{
			Subject: "user-1", Scheme: authkit.SchemeJWT, Claims: authkit.Claims{"tenant": "t-1"},
		}, mustClaimSet("tenant")), metadata)

		from := initiator.From(propagator.Extract(context.Background(), metadata))

		Expect(from.Kind()).To(Equal(initiator.KindAppUser))
		Expect(from.ID()).To(Equal("user-1"))
		Expect(from.Claims()).To(Equal(authkit.Claims{"tenant": "t-1"}))
	})

	// A job enqueued before propagators existed has no metadata at all.
	It("reads no metadata as unknown", func() {
		from := initiator.From(initiator.NewPropagator(nil, "worker job").Extract(context.Background(), nil))

		Expect(from.Kind()).To(Equal(initiator.KindUnknown))
	})

	It("names where the value arrived in what it reports", func() {
		records := captureLogRecords(func() {
			initiator.NewPropagator(nil, "worker job").Extract(context.Background(), map[string]string{initiator.HeaderID: "user-1"})
		})

		warnings := recordsOfType(records, initiator.LogTypeHeaderMalformed)
		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).To(HaveKeyWithValue("source", "worker job"))
	})
})
