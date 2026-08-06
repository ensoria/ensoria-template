package mb_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"

	mbApp "github.com/ensoria/ensoria-template/internal/app/mb"
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
