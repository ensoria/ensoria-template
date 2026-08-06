package mbkit_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
)

// published records one call reaching the broker.
type published struct {
	ctx    context.Context
	target string
	data   []byte
	meta   map[string]string
	opts   []mb.PublishOption
	calls  int
	err    error
}

// broker returns an mb.Publish that records instead of connecting.
func (p *published) broker() mb.Publish {
	return func(ctx context.Context, target string, data []byte, metadata map[string]string, opts ...mb.PublishOption) error {
		p.calls++
		p.ctx = ctx
		p.target = target
		p.data = data
		p.meta = metadata
		p.opts = opts
		return p.err
	}
}

// newPublication binds a declaration to the recording broker.
func newPublication(rec *published, adjust func(*mbkit.PublicationSpec[order])) *mbkit.Publication[order] {
	spec := &mbkit.PublicationSpec[order]{
		Target:  "orders",
		Summary: "Announce an order",
	}
	if adjust != nil {
		adjust(spec)
	}
	return mbkit.NewPublication(rec.broker(), spec)
}

var _ = Describe("Publication Publish", func() {
	It("encodes the message and sends it to the declared target", func() {
		rec := &published{}
		pub := newPublication(rec, nil)

		err := pub.Publish(context.Background(), &order{ID: "ord_1", Amount: 120}, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(rec.calls).To(Equal(1))
		Expect(rec.target).To(Equal("orders"))

		var sent order
		Expect(json.Unmarshal(rec.data, &sent)).To(Succeed())
		Expect(sent).To(Equal(order{ID: "ord_1", Amount: 120}))
	})

	It("passes the caller's context and metadata through", func() {
		type ctxKey struct{}
		rec := &published{}
		pub := newPublication(rec, nil)
		ctx := context.WithValue(context.Background(), ctxKey{}, "trace-1")

		Expect(pub.Publish(ctx, &order{ID: "ord_1"}, mbkit.Metadata{"source": "spec"})).To(Succeed())

		Expect(rec.ctx.Value(ctxKey{})).To(Equal("trace-1"))
		Expect(rec.meta).To(HaveKeyWithValue("source", "spec"))
	})

	It("returns the broker's error unchanged", func() {
		brokerErr := errors.New("broker unreachable")
		rec := &published{err: brokerErr}
		pub := newPublication(rec, nil)

		err := pub.Publish(context.Background(), &order{ID: "ord_1"}, nil)

		Expect(err).To(MatchError(brokerErr))
	})

	Describe("options", func() {
		It("applies the declared options to every call", func() {
			rec := &published{}
			pub := newPublication(rec, func(p *mbkit.PublicationSpec[order]) {
				p.Options = []mb.PublishOption{mb.WithPriority(5)}
			})

			Expect(pub.Publish(context.Background(), &order{ID: "ord_1"}, nil)).To(Succeed())

			Expect(mbkit.ResolvePublishConfig(rec.opts).Priority).To(BeEquivalentTo(5))
		})

		It("lets a per-call option override a declared one", func() {
			rec := &published{}
			pub := newPublication(rec, func(p *mbkit.PublicationSpec[order]) {
				p.Options = []mb.PublishOption{mb.WithPriority(5)}
			})

			Expect(pub.Publish(context.Background(), &order{ID: "ord_1"}, nil, mb.WithPriority(9))).To(Succeed())

			// Per-call options are applied last, so the call wins over the
			// declared default without changing it for anyone else.
			Expect(mbkit.ResolvePublishConfig(rec.opts).Priority).To(BeEquivalentTo(9))
		})

		It("does not let a per-call option leak into the next call", func() {
			rec := &published{}
			pub := newPublication(rec, func(p *mbkit.PublicationSpec[order]) {
				p.Options = []mb.PublishOption{mb.WithPriority(5)}
			})

			Expect(pub.Publish(context.Background(), &order{ID: "ord_1"}, nil, mb.WithPriority(9))).To(Succeed())
			Expect(pub.Publish(context.Background(), &order{ID: "ord_2"}, nil)).To(Succeed())

			Expect(mbkit.ResolvePublishConfig(rec.opts).Priority).To(BeEquivalentTo(5))
		})
	})

	Describe("a message that fails its validation rules", func() {
		It("is not published at all", func() {
			rec := &published{}
			pub := newPublication(rec, func(p *mbkit.PublicationSpec[order]) {
				p.BodyRules = []*rule.RuleSet{
					{Field: "id", Rules: []rule.Rule{vkit.Required()}},
				}
			})

			err := pub.Publish(context.Background(), &order{Amount: 120}, nil)

			// Emitting a message that violates the contract this declaration
			// documents is the one failure that can still be stopped here.
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("id"))
			Expect(rec.calls).To(BeZero())
		})
	})

	// A declaration cannot be injected in place of a publication: writing
	// &mbkit.Publication[order]{Target: "orders"} does not compile, because the
	// declared fields live on PublicationSpec. Only the degenerate empty literal
	// remains constructible, and it is refused rather than left to panic.
	It("refuses to publish when it was not built by NewPublication", func() {
		unbuilt := &mbkit.Publication[order]{}

		err := unbuilt.Publish(context.Background(), &order{ID: "ord_1"}, nil)

		Expect(err).To(MatchError(ContainSubstring("NewPublication")))
		Expect(unbuilt.Target()).To(BeEmpty())
	})

	It("reports the target it was declared with", func() {
		Expect(newPublication(&published{}, nil).Target()).To(Equal("orders"))
	})
})

var _ = Describe("PublicationDoc", func() {
	It("carries the declaration through in a non-generic shape", func() {
		idempotent := true
		rec := &published{}
		pub := newPublication(rec, func(p *mbkit.PublicationSpec[order]) {
			p.Description = "Longer explanation"
			p.Task = "announce order"
			p.FieldDocs = map[string]string{"id": "identifier of the order"}
			p.Behavior = mbkit.BehaviorSpec{
				SideEffects: []string{"none"},
				Idempotent:  &idempotent,
				Delivery:    mbkit.DeliverySpec{Guarantee: "at-least-once"},
			}
		})

		doc := pub.PublicationDoc()

		Expect(doc.Target).To(Equal("orders"))
		Expect(doc.Summary).To(Equal("Announce an order"))
		Expect(doc.Task).To(Equal("announce order"))
		Expect(doc.FieldDocs).To(HaveKeyWithValue("id", "identifier of the order"))
		Expect(doc.MsgType).To(Equal(reflect.TypeFor[order]()))
		Expect(doc.Behavior.Idempotent).To(Equal(&idempotent))
	})

	It("resolves the declared options into the configuration really in force", func() {
		rec := &published{}
		pub := newPublication(rec, func(p *mbkit.PublicationSpec[order]) {
			p.Options = []mb.PublishOption{mb.WithPriority(5), mb.WithExpiration("60000")}
		})

		cfg := pub.PublicationDoc().Config

		Expect(cfg.Priority).To(BeEquivalentTo(5))
		Expect(cfg.Expiration).To(Equal("60000"))
		Expect(cfg.DeliveryMode).To(Equal(mb.DefaultPublishConfig().DeliveryMode))
	})

	It("is reachable through the documentation-only interface the group holds", func() {
		rec := &published{}
		pub := newPublication(rec, nil)

		var documented mbkit.DocumentedPublication = mbkit.AsPublicationDoc(pub)

		Expect(documented.PublicationDoc().Target).To(Equal("orders"))
	})
})

var _ = Describe("ResolveSubscribeConfig", func() {
	It("returns the broker defaults when nothing was declared", func() {
		Expect(mbkit.ResolveSubscribeConfig(nil)).To(Equal(mb.DefaultSubscribeConfig()))
	})

	It("does not let one subscription's options affect another's", func() {
		first := mbkit.ResolveSubscribeConfig([]mb.SubscribeOption{mb.WithQueueGroup("a")})
		second := mbkit.ResolveSubscribeConfig(nil)

		Expect(first.QueueGroup).To(Equal("a"))
		Expect(second.QueueGroup).To(BeEmpty())
	})
})
