package mb

import (
	"context"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
)

// HelloWorldTarget is the channel this module both consumes and publishes to.
// Declaring it once keeps the subscription and the publication from drifting
// onto two channels whose names merely look alike.
const HelloWorldTarget = "hello_world"

// NewHelloWorldSubscription declares the subscription to the hello_world
// channel.
//
// The subscribe target and its options are declared here rather than passed to
// a subscribe call at startup, which is what lets the AsyncAPI generator learn
// that this channel exists and what arrives on it.
func NewHelloWorldSubscription(us service.UserService) *mbkit.SubscriptionModule {
	return mbkit.NewSubscriptionModule(&mbkit.Subscription[dto.HelloWorld]{
		Target:      HelloWorldTarget,
		Summary:     "Consume the demonstration hello_world message",
		Description: "Sample subscription showing a typed message, its validation rules and its delivery settings.",
		Task:        "demo receive",
		FieldDocs: map[string]string{
			"message": "free-form text carried by the message",
			"source":  "name of the code path that published it",
		},
		BodyRules: []*rule.RuleSet{
			{Field: "message", Rules: []rule.Rule{vkit.Required()}},
		},
		Behavior: mbkit.BehaviorSpec{
			SideEffects:   []string{"writes a log line"},
			Idempotent:    new(true),
			Preconditions: []string{"none"},
			Ordering:      "none",
			Delivery: mbkit.DeliverySpec{
				Guarantee: "at-most-once",
				Notes: []string{
					"Errors discard the message rather than requeue it, so a failing handler never blocks the queue.",
				},
			},
		},
		Options: []mb.SubscribeOption{
			mb.WithErrorStrategy(mb.ErrorStrategyDiscard),
		},
		Handle: func(ctx context.Context, msg *dto.HelloWorld, meta mbkit.Metadata) error {
			loggear.Info("📨 Received message",
				"topic", meta["topic"],
				"partition", meta["partition"],
				"offset", meta["offset"],
				"key", meta["key"],
				"message", msg.Message,
				"source", msg.Source,
				"timestamp", meta["timestamp"])
			return nil
		},
	})
}
