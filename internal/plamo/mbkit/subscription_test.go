package mbkit_test

import (
	"context"
	"errors"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
)

// order is the message type these specs subscribe to.
type order struct {
	ID     string `json:"id"`
	Amount int    `json:"amount"`
}

// received records what the handler was given, so the specs can assert on the
// decoded value rather than on the raw bytes.
type received struct {
	msg   *order
	meta  mbkit.Metadata
	calls int
}

// newSubscription builds a module around a recording handler, applying any
// per-spec adjustments to the declaration first.
func newSubscription(rec *received, adjust func(*mbkit.Subscription[order])) *mbkit.SubscriptionModule {
	sub := &mbkit.Subscription[order]{
		Target: "orders",
		Handle: func(ctx context.Context, msg *order, meta mbkit.Metadata) error {
			rec.calls++
			rec.msg = msg
			rec.meta = meta
			return nil
		},
	}
	if adjust != nil {
		adjust(sub)
	}
	return mbkit.NewSubscriptionModule(sub)
}

var _ = Describe("SubscriptionModule OnReceive", func() {
	It("decodes the payload and hands the typed message to the handler", func() {
		rec := &received{}
		module := newSubscription(rec, nil)

		err := module.OnReceive(context.Background(),
			[]byte(`{"id":"ord_1","amount":120}`),
			map[string]string{"topic": "orders"})

		Expect(err).NotTo(HaveOccurred())
		Expect(rec.calls).To(Equal(1))
		Expect(rec.msg.ID).To(Equal("ord_1"))
		Expect(rec.msg.Amount).To(Equal(120))
		Expect(rec.meta).To(HaveKeyWithValue("topic", "orders"))
	})

	It("returns the handler's error unchanged", func() {
		handlerErr := errors.New("downstream unavailable")
		module := mbkit.NewSubscriptionModule(&mbkit.Subscription[order]{
			Target: "orders",
			Handle: func(context.Context, *order, mbkit.Metadata) error { return handlerErr },
		})

		err := module.OnReceive(context.Background(), []byte(`{"id":"ord_1"}`), nil)

		// The broker's ErrorStrategy decides what happens next, so the error has
		// to reach it intact rather than being wrapped or swallowed.
		Expect(err).To(MatchError(handlerErr))
	})

	It("propagates the receive context to the handler", func() {
		type ctxKey struct{}
		var got context.Context
		module := mbkit.NewSubscriptionModule(&mbkit.Subscription[order]{
			Target: "orders",
			Handle: func(ctx context.Context, _ *order, _ mbkit.Metadata) error {
				got = ctx
				return nil
			},
		})
		ctx := context.WithValue(context.Background(), ctxKey{}, "trace-1")

		Expect(module.OnReceive(ctx, []byte(`{"id":"ord_1"}`), nil)).To(Succeed())

		Expect(got.Value(ctxKey{})).To(Equal("trace-1"))
	})

	Describe("a message that cannot be decoded", func() {
		It("discards it without calling the handler, by default", func() {
			rec := &received{}
			module := newSubscription(rec, nil)

			err := module.OnReceive(context.Background(), []byte(`{"id":`), nil)

			// Returning an error here would requeue a message that can never
			// succeed, stalling the queue behind it.
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.calls).To(BeZero())
		})

		It("reports it to the broker when the declaration asks for that", func() {
			rec := &received{}
			module := newSubscription(rec, func(s *mbkit.Subscription[order]) {
				s.OnInvalid = mbkit.InvalidFail
			})

			err := module.OnReceive(context.Background(), []byte(`{"id":`), nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("orders"))
			Expect(rec.calls).To(BeZero())
		})
	})

	Describe("a message that fails its validation rules", func() {
		requireID := func(s *mbkit.Subscription[order]) {
			s.BodyRules = []*rule.RuleSet{
				{Field: "id", Rules: []rule.Rule{vkit.Required()}},
			}
		}

		It("never reaches the handler", func() {
			rec := &received{}
			module := newSubscription(rec, requireID)

			err := module.OnReceive(context.Background(), []byte(`{"amount":120}`), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(rec.calls).To(BeZero())
		})

		It("names the offending field when it is reported to the broker", func() {
			rec := &received{}
			module := newSubscription(rec, func(s *mbkit.Subscription[order]) {
				requireID(s)
				s.OnInvalid = mbkit.InvalidFail
			})

			err := module.OnReceive(context.Background(), []byte(`{"amount":120}`), nil)

			// A dead-letter consumer should learn which field was wrong, not
			// merely that the message was bad.
			Expect(err.Error()).To(ContainSubstring("id"))
			Expect(err.Error()).To(ContainSubstring("str_not_empty"))
		})

		It("lets a message satisfying the rules through", func() {
			rec := &received{}
			module := newSubscription(rec, requireID)

			Expect(module.OnReceive(context.Background(), []byte(`{"id":"ord_1"}`), nil)).To(Succeed())

			Expect(rec.calls).To(Equal(1))
		})
	})
})

var _ = Describe("SubscriptionDoc", func() {
	It("carries the declaration through in a non-generic shape", func() {
		idempotent := true
		module := mbkit.NewSubscriptionModule(&mbkit.Subscription[order]{
			Target:      "orders",
			Summary:     "Consume orders",
			Description: "Longer explanation",
			Task:        "sync order",
			AlsoRead:    []string{"workflows/ordering.md"},
			Related:     []string{"Emitted by: POST /orders"},
			FieldDocs:   map[string]string{"id": "identifier of the order"},
			BodyRules: []*rule.RuleSet{
				{Field: "id", Rules: []rule.Rule{vkit.Required()}},
			},
			Behavior: mbkit.BehaviorSpec{
				SideEffects: []string{"writes a row"},
				Idempotent:  &idempotent,
				Ordering:    "none",
				Delivery:    mbkit.DeliverySpec{Guarantee: "at-least-once"},
			},
			Handle: func(context.Context, *order, mbkit.Metadata) error { return nil },
		})

		doc := module.SubscriptionDoc()

		Expect(doc.Target).To(Equal("orders"))
		Expect(doc.Summary).To(Equal("Consume orders"))
		Expect(doc.Task).To(Equal("sync order"))
		Expect(doc.AlsoRead).To(Equal([]string{"workflows/ordering.md"}))
		Expect(doc.Related).To(Equal([]string{"Emitted by: POST /orders"}))
		Expect(doc.FieldDocs).To(HaveKeyWithValue("id", "identifier of the order"))
		Expect(doc.BodyRules).To(HaveLen(1))
		Expect(doc.Behavior.Idempotent).To(Equal(&idempotent))
		Expect(doc.Behavior.Delivery.Guarantee).To(Equal("at-least-once"))
	})

	It("records the message type, which is where the payload schema comes from", func() {
		module := newSubscription(&received{}, nil)

		Expect(module.SubscriptionDoc().MsgType).To(Equal(reflect.TypeFor[order]()))
	})

	It("resolves the declared options into the configuration really in force", func() {
		module := newSubscription(&received{}, func(s *mbkit.Subscription[order]) {
			s.Options = []mb.SubscribeOption{
				mb.WithErrorStrategy(mb.ErrorStrategyDiscard),
				mb.WithQueueGroup("orders-workers"),
			}
		})

		cfg := module.SubscriptionDoc().Config

		Expect(cfg.ErrorStrategy).To(Equal(mb.ErrorStrategyDiscard))
		Expect(cfg.QueueGroup).To(Equal("orders-workers"))
		// Untouched settings keep the broker defaults, which the document should
		// state rather than leave the reader to guess.
		Expect(cfg.MaxRetries).To(Equal(mb.DefaultSubscribeConfig().MaxRetries))
		Expect(cfg.AutoAck).To(BeFalse())
	})

	It("exposes the target and options the app layer subscribes with", func() {
		opts := []mb.SubscribeOption{mb.WithAutoAck()}
		module := newSubscription(&received{}, func(s *mbkit.Subscription[order]) {
			s.Options = opts
		})

		Expect(module.Target()).To(Equal("orders"))
		Expect(module.Options()).To(HaveLen(len(opts)))
	})
})
