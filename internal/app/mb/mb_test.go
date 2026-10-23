package mb_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"

	mbApp "github.com/ensoria/ensoria-template/internal/app/mb"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/mb/pkg/mb"
)

// fakeLifecycle collects the hooks instead of running an fx application, so a
// spec can trigger OnStart itself.
type fakeLifecycle struct {
	hooks []fx.Hook
}

func (l *fakeLifecycle) Append(h fx.Hook) { l.hooks = append(l.hooks, h) }

// start runs every registered OnStart hook, stopping at the first failure, the
// way fx does.
func (l *fakeLifecycle) start(ctx context.Context) error {
	for _, h := range l.hooks {
		if h.OnStart == nil {
			continue
		}
		if err := h.OnStart(ctx); err != nil {
			return err
		}
	}
	return nil
}

// subscribeCall records one subscription the app layer started.
type subscribeCall struct {
	target string
	opts   []mb.SubscribeOption
}

// recorder is a StartSubscription that records instead of connecting.
type recorder struct {
	calls []subscribeCall
	err   error
}

func (r *recorder) start() mb.StartSubscription {
	return func(target string, handler mb.SubscribeHandler, opts ...mb.SubscribeOption) error {
		r.calls = append(r.calls, subscribeCall{target: target, opts: opts})
		return r.err
	}
}

// subscription builds a module for one target, with the given options.
func subscription(target string, opts ...mb.SubscribeOption) *mbkit.SubscriptionModule {
	return mbkit.NewSubscriptionModule(&mbkit.Subscription[struct{}]{
		Target:  target,
		Options: opts,
		Handle:  func(context.Context, *struct{}, mbkit.Metadata) error { return nil },
	})
}

var _ = Describe("StartSubscriptions", func() {
	It("subscribes every declared module when the application starts", func() {
		lc := &fakeLifecycle{}
		rec := &recorder{}
		modules := []*mbkit.SubscriptionModule{
			subscription("orders"),
			subscription("users"),
		}

		mbApp.StartSubscriptions(lc, rec.start(), modules)

		// Nothing is subscribed at wiring time: a subscription started before
		// the application is ready would receive messages it cannot yet handle.
		Expect(rec.calls).To(BeEmpty())

		Expect(lc.start(context.Background())).To(Succeed())

		Expect(rec.calls).To(HaveLen(2))
		Expect(rec.calls[0].target).To(Equal("orders"))
		Expect(rec.calls[1].target).To(Equal("users"))
	})

	It("subscribes with the options the declaration carries", func() {
		lc := &fakeLifecycle{}
		rec := &recorder{}
		modules := []*mbkit.SubscriptionModule{
			subscription("orders", mb.WithErrorStrategy(mb.ErrorStrategyDiscard)),
		}

		mbApp.StartSubscriptions(lc, rec.start(), modules)
		Expect(lc.start(context.Background())).To(Succeed())

		cfg := mbkit.ResolveSubscribeConfig(rec.calls[0].opts)
		Expect(cfg.ErrorStrategy).To(Equal(mb.ErrorStrategyDiscard))
	})

	It("fails startup, naming the channel, when a subscription cannot start", func() {
		lc := &fakeLifecycle{}
		rec := &recorder{err: errors.New("broker unreachable")}

		mbApp.StartSubscriptions(lc, rec.start(), []*mbkit.SubscriptionModule{subscription("orders")})
		err := lc.start(context.Background())

		// Starting anyway would leave the application silently not consuming a
		// channel it is documented as consuming.
		Expect(err).To(MatchError(ContainSubstring("orders")))
		Expect(err).To(MatchError(ContainSubstring("broker unreachable")))
	})

	It("starts cleanly when no subscription is declared", func() {
		lc := &fakeLifecycle{}
		rec := &recorder{}

		mbApp.StartSubscriptions(lc, rec.start(), nil)

		Expect(lc.start(context.Background())).To(Succeed())
		Expect(rec.calls).To(BeEmpty())
	})
})

// entryContext is a request context the way the HTTP middleware leaves it.
func entryContext(caller *authkit.Principal, claims *initiator.ClaimSet) context.Context {
	ctx := initiator.WithEntry(context.Background(), claims)
	if caller != nil {
		ctx = authkit.WithPrincipal(ctx, caller)
	}
	return ctx
}

func mustClaimSet(names ...string) *initiator.ClaimSet {
	GinkgoHelper()

	set, err := initiator.NewClaimSet(names)
	Expect(err).NotTo(HaveOccurred())
	return set
}

var _ = Describe("NewPublish", func() {
	It("writes who started the work into the message's headers", func() {
		pub := &fakePublisher{carries: true}
		publish := mbApp.NewPublish(pub)
		ctx := entryContext(&authkit.Principal{
			Subject: "user-1", Scheme: authkit.SchemeSession, Claims: authkit.Claims{"tenant": "t-1"},
		}, mustClaimSet("tenant"))

		Expect(publish(ctx, "orders", []byte(`{}`), nil)).To(Succeed())

		Expect(pub.sent).To(HaveLen(1))
		Expect(pub.sent[0].metadata).To(Equal(map[string]string{
			initiator.HeaderKind:   string(initiator.KindAppUser),
			initiator.HeaderID:     "user-1",
			initiator.HeaderClaims: `{"tenant":"t-1"}`,
		}))
	})

	It("writes unknown when nothing says who started the work", func() {
		pub := &fakePublisher{carries: true}

		Expect(mbApp.NewPublish(pub)(context.Background(), "orders", []byte(`{}`), nil)).To(Succeed())

		Expect(pub.sent[0].metadata).To(Equal(map[string]string{initiator.HeaderKind: string(initiator.KindUnknown)}))
	})

	It("keeps the application's own metadata, without changing the map it was given", func() {
		pub := &fakePublisher{carries: true}
		metadata := map[string]string{"trace_id": "t-1"}

		Expect(mbApp.NewPublish(pub)(initiator.WithSystem(context.Background()), "orders", []byte(`{}`), metadata)).To(Succeed())

		Expect(pub.sent[0].metadata).To(Equal(map[string]string{
			"trace_id":           "t-1",
			initiator.HeaderKind: string(initiator.KindSystem),
		}))
		Expect(metadata).To(Equal(map[string]string{"trace_id": "t-1"}))
	})

	// The framework writes these from the context; a hand-written value next to
	// it would silently send a different origin than the one the work has.
	DescribeTable("refuses metadata that names one of the reserved headers",
		func(key string) {
			pub := &fakePublisher{carries: true}

			err := mbApp.NewPublish(pub)(context.Background(), "orders", []byte(`{}`), map[string]string{key: "hand-written"})

			Expect(err).To(MatchError(mbApp.ErrReservedMetadata))
			Expect(err).To(MatchError(ContainSubstring(key)))
			Expect(pub.sent).To(BeEmpty())
		},
		Entry("the kind", initiator.HeaderKind),
		Entry("the ID", initiator.HeaderID),
		Entry("the claims", initiator.HeaderClaims),
	)

	Describe("on a broker that cannot carry metadata", func() {
		It("leaves who started the work off, rather than having every publish refused", func() {
			pub := &fakePublisher{carries: false}

			Expect(mbApp.NewPublish(pub)(initiator.WithSystem(context.Background()), "orders", []byte(`{}`), map[string]string{"key": "user-1"})).To(Succeed())

			Expect(pub.sent[0].metadata).To(Equal(map[string]string{"key": "user-1"}))
		})

		It("says so once, when the publisher is built", func() {
			pub := &fakePublisher{carries: false}

			records := captureLogRecords(func() {
				publish := mbApp.NewPublish(pub)
				_ = publish(context.Background(), "orders", []byte(`{}`), nil)
				_ = publish(context.Background(), "orders", []byte(`{}`), nil)
			})

			Expect(recordsOfType(records, mbApp.LogTypeInitiatorNotCarried)).To(HaveLen(1))
		})

		It("still refuses the reserved headers", func() {
			pub := &fakePublisher{carries: false}

			err := mbApp.NewPublish(pub)(context.Background(), "orders", []byte(`{}`), map[string]string{initiator.HeaderKind: "system"})

			Expect(err).To(MatchError(mbApp.ErrReservedMetadata))
		})

		It("says nothing on a broker that can", func() {
			records := captureLogRecords(func() { mbApp.NewPublish(&fakePublisher{carries: true}) })

			Expect(recordsOfType(records, mbApp.LogTypeInitiatorNotCarried)).To(BeEmpty())
		})
	})
})

var _ = Describe("NewSubscribe", func() {
	// received is what the subscription handler was given.
	type received struct {
		ctx      context.Context
		metadata map[string]string
	}

	subscribe := func(sub *fakeSubscriber, claims *initiator.ClaimSet, onReceive func(context.Context, map[string]string)) {
		start := mbApp.NewSubscribe(dikit.RootContext{Ctx: context.Background()}, sub, claims)
		Expect(start("orders", handlerFunc(func(ctx context.Context, _ []byte, metadata map[string]string) error {
			onReceive(ctx, metadata)
			return nil
		}))).To(Succeed())
	}

	It("restores who started the work onto the handler's context", func() {
		sub := newFakeSubscriber()
		var got received
		subscribe(sub, mustClaimSet("tenant"), func(ctx context.Context, md map[string]string) { got = received{ctx, md} })

		Expect(sub.deliver("orders", map[string]string{
			"routing_key":          "orders",
			initiator.HeaderKind:   string(initiator.KindAppUser),
			initiator.HeaderID:     "user-1",
			initiator.HeaderClaims: `{"tenant":"t-1","email":"x@example.com"}`,
		})).To(Succeed())

		from := initiator.From(got.ctx)
		Expect(from.Kind()).To(Equal(initiator.KindAppUser))
		Expect(from.ID()).To(Equal("user-1"))
		Expect(from.Claims()).To(Equal(authkit.Claims{"tenant": "t-1"}))
	})

	// The one way to read them is initiator.From, so that nothing reads the
	// raw headers past the rules FromHeaders applies.
	It("hands the handler the metadata without the reserved headers", func() {
		sub := newFakeSubscriber()
		var got received
		subscribe(sub, nil, func(ctx context.Context, md map[string]string) { got = received{ctx, md} })

		Expect(sub.deliver("orders", map[string]string{
			"routing_key":        "orders",
			"trace_id":           "t-1",
			initiator.HeaderKind: string(initiator.KindSystem),
		})).To(Succeed())

		Expect(got.metadata).To(Equal(map[string]string{"routing_key": "orders", "trace_id": "t-1"}))
	})

	It("reads a message from a producer that does not follow the convention as unknown", func() {
		sub := newFakeSubscriber()
		var got received
		subscribe(sub, nil, func(ctx context.Context, md map[string]string) { got = received{ctx, md} })

		Expect(sub.deliver("orders", map[string]string{"routing_key": "orders"})).To(Succeed())

		Expect(initiator.From(got.ctx).Kind()).To(Equal(initiator.KindUnknown))
	})

	// Context, not a gate: a malformed value never stops the message.
	It("still hands a message with a malformed value to the handler, as unknown", func() {
		sub := newFakeSubscriber()
		var got *received
		subscribe(sub, nil, func(ctx context.Context, md map[string]string) { got = &received{ctx, md} })

		Expect(sub.deliver("orders", map[string]string{initiator.HeaderID: "user-1"})).To(Succeed())

		Expect(got).NotTo(BeNil())
		Expect(initiator.From(got.ctx).Kind()).To(Equal(initiator.KindUnknown))
	})

	It("passes the work on to what the handler publishes next", func() {
		sub := newFakeSubscriber()
		pub := &fakePublisher{carries: true}
		publish := mbApp.NewPublish(pub)
		subscribe(sub, mustClaimSet("tenant"), func(ctx context.Context, _ map[string]string) {
			Expect(publish(ctx, "invoices", []byte(`{}`), nil)).To(Succeed())
		})

		Expect(sub.deliver("orders", map[string]string{
			initiator.HeaderKind:   string(initiator.KindAppUser),
			initiator.HeaderID:     "user-1",
			initiator.HeaderClaims: `{"tenant":"t-1","email":"x@example.com"}`,
		})).To(Succeed())

		// The claim this application does not carry is not sent on.
		Expect(pub.sent).To(HaveLen(1))
		Expect(pub.sent[0].metadata).To(Equal(map[string]string{
			initiator.HeaderKind:   string(initiator.KindAppUser),
			initiator.HeaderID:     "user-1",
			initiator.HeaderClaims: `{"tenant":"t-1"}`,
		}))
	})
})
