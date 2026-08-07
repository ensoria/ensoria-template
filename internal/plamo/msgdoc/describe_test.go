package msgdoc_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/websocket/pkg/wsevent"
)

// subscriptionDoc builds a subscription declaration, applying any per-spec
// adjustment before it is converted.
func subscriptionDoc(adjust func(*mbkit.Subscription[userCreated])) *mbkit.SubscriptionDoc {
	sub := &mbkit.Subscription[userCreated]{
		Target:  "user.created",
		Summary: "Consume the user-created event",
		Handle:  func(context.Context, *userCreated, mbkit.Metadata) error { return nil },
	}
	if adjust != nil {
		adjust(sub)
	}
	return mbkit.NewSubscriptionModule(sub).SubscriptionDoc()
}

// publicationDoc builds a publication declaration.
func publicationDoc(adjust func(*mbkit.PublicationSpec[userCreated])) *mbkit.PublicationDoc {
	spec := &mbkit.PublicationSpec[userCreated]{
		Target:  "user.created",
		Summary: "Announce that a user was created",
	}
	if adjust != nil {
		adjust(spec)
	}
	publish := func(context.Context, string, []byte, map[string]string, ...mb.PublishOption) error { return nil }
	return mbkit.NewPublication(publish, spec).PublicationDoc()
}

var _ = Describe("DescribeSubscription", func() {
	It("writes the subscription as a receive operation on its channel", func() {
		op := msgdoc.DescribeSubscription(subscriptionDoc(nil), "rabbitmq", []string{"broker"})

		Expect(op.Action).To(Equal(msgdoc.ActionReceive))
		Expect(op.Protocol).To(Equal("rabbitmq"))
		Expect(op.Channel.Address).To(Equal("user.created"))
		Expect(op.Channel.ServerNames).To(Equal([]string{"broker"}))
		Expect(op.Summary).To(Equal("Consume the user-created event"))
		Expect(op.Untyped).To(BeFalse())
	})

	It("describes the payload from the declared type, rules and field docs", func() {
		doc := subscriptionDoc(func(s *mbkit.Subscription[userCreated]) {
			s.BodyRules = []*rule.RuleSet{
				{Field: "name", Rules: []rule.Rule{vkit.Required()}},
			}
			s.FieldDocs = map[string]string{"id": "identifier of the created user"}
		})

		op := msgdoc.DescribeSubscription(doc, "rabbitmq", nil)

		Expect(op.Messages).To(HaveLen(1))
		message := op.Messages[0]
		Expect(message.Name).To(Equal("userCreated"))
		Expect(message.ContentType).To(Equal("application/json"))
		Expect(message.Payload.Type).To(Equal(apidoc.TypeObject))
		Expect(message.Payload.Fields[0].Meaning).To(Equal("identifier of the created user"))
		Expect(message.Payload.Fields[1].Required).To(BeTrue())
		// An example is what lets a reader see the message rather than assemble
		// it from a field table.
		Expect(message.Payload.Example).NotTo(BeNil())
	})

	It("keeps both the declared guarantee and the settings really in force", func() {
		doc := subscriptionDoc(func(s *mbkit.Subscription[userCreated]) {
			s.Behavior.Delivery = mbkit.DeliverySpec{Guarantee: "at-most-once"}
			s.Options = []mb.SubscribeOption{mb.WithErrorStrategy(mb.ErrorStrategyDiscard)}
		})

		delivery := msgdoc.DescribeSubscription(doc, "rabbitmq", nil).Behavior.Delivery

		// The promise and the configuration meant to keep it can disagree, so a
		// document that showed only one of them would hide the disagreement.
		Expect(delivery.Guarantee).To(Equal("at-most-once"))
		Expect(delivery.Resolved).To(HaveKeyWithValue(msgdoc.DeliveryErrorStrategy, "discard"))
	})

	Describe("the settings it reports per broker family", func() {
		It("reports queue settings for a queue broker", func() {
			resolved := msgdoc.DescribeSubscription(subscriptionDoc(nil), "rabbitmq", nil).Behavior.Delivery.Resolved

			Expect(resolved).To(HaveKey(msgdoc.DeliveryDurable))
			Expect(resolved).To(HaveKey(msgdoc.DeliveryExclusive))
			// mb fills every field of SubscribeConfig with a default, streaming
			// ones included. Reporting them would document this queue as having
			// a consumer group the broker never reads.
			Expect(resolved).NotTo(HaveKey(msgdoc.DeliveryConsumerGroup))
			Expect(resolved).NotTo(HaveKey(msgdoc.DeliveryStartOffset))
		})

		It("reports stream settings for a streaming broker", func() {
			resolved := msgdoc.DescribeSubscription(subscriptionDoc(nil), "kafka", nil).Behavior.Delivery.Resolved

			Expect(resolved).To(HaveKey(msgdoc.DeliveryConsumerGroup))
			Expect(resolved).To(HaveKeyWithValue(msgdoc.DeliveryStartOffset, "stored"))
			Expect(resolved).NotTo(HaveKey(msgdoc.DeliveryDurable))
		})

		It("reports the retry budget only when retrying is the strategy", func() {
			retrying := subscriptionDoc(func(s *mbkit.Subscription[userCreated]) {
				s.Options = []mb.SubscribeOption{
					mb.WithErrorStrategy(mb.ErrorStrategyRetry),
					mb.WithMaxRetries(7),
				}
			})

			Expect(msgdoc.DescribeSubscription(retrying, "rabbitmq", nil).Behavior.Delivery.Resolved).
				To(HaveKeyWithValue(msgdoc.DeliveryMaxRetries, "7"))
			// Under any other strategy it is a number that never applies.
			Expect(msgdoc.DescribeSubscription(subscriptionDoc(nil), "rabbitmq", nil).Behavior.Delivery.Resolved).
				NotTo(HaveKey(msgdoc.DeliveryMaxRetries))
		})
	})
})

var _ = Describe("DescribePublication", func() {
	It("writes the publication as a send operation on its channel", func() {
		op := msgdoc.DescribePublication(publicationDoc(nil), "rabbitmq", []string{"broker"})

		Expect(op.Action).To(Equal(msgdoc.ActionSend))
		Expect(op.Protocol).To(Equal("rabbitmq"))
		Expect(op.Channel.Address).To(Equal("user.created"))
		Expect(op.Messages[0].Name).To(Equal("userCreated"))
	})

	It("reports the message attributes of a queue broker", func() {
		doc := publicationDoc(func(p *mbkit.PublicationSpec[userCreated]) {
			p.Options = []mb.PublishOption{mb.WithPersistentDelivery(), mb.WithExpiration("60000")}
		})

		resolved := msgdoc.DescribePublication(doc, "rabbitmq", nil).Behavior.Delivery.Resolved

		Expect(resolved).To(HaveKeyWithValue(msgdoc.DeliveryDeliveryMode, "persistent"))
		Expect(resolved).To(HaveKeyWithValue(msgdoc.DeliveryExpiration, "60000"))
	})

	It("does not put the operation's wording on the message", func() {
		receive := msgdoc.DescribeSubscription(subscriptionDoc(nil), "rabbitmq", nil)
		send := msgdoc.DescribePublication(publicationDoc(nil), "rabbitmq", nil)

		// Both directions of a channel produce the same message, and a renderer
		// keeps one entry per message name. If the operation's summary were
		// copied onto it, whichever direction rendered last would leave its
		// wording — "Announce that a user was created" — showing for the other.
		Expect(receive.Messages[0].Name).To(Equal(send.Messages[0].Name))
		Expect(receive.Messages[0].Summary).To(BeEmpty())
		Expect(send.Messages[0].Summary).To(BeEmpty())
		// The prose is not lost: it stays on the operation, where it belongs.
		Expect(receive.Summary).To(Equal("Consume the user-created event"))
		Expect(send.Summary).To(Equal("Announce that a user was created"))
	})

	It("reports nothing for a stream, which carries none of those attributes", func() {
		Expect(msgdoc.DescribePublication(publicationDoc(nil), "kafka", nil).Behavior.Delivery.Resolved).
			To(BeEmpty())
	})
})

var _ = Describe("DescribeChannel", func() {
	// wsChannel declares a channel with one message in each direction.
	wsChannel := func(adjust func(*wskit.Channel)) *wskit.ModuleDoc {
		channel := &wskit.Channel{
			Path:    "/ws/user",
			Summary: "User real-time channel",
			Receive: []*wskit.Receiver{
				wskit.Receive[userCreated]("user.echo", wskit.MessageOpts{Summary: "Echo request"},
					func(context.Context, *wsevent.Message, *userCreated) error { return nil }),
			},
			Send: []wskit.DocumentedMessage{
				wskit.Send[userCreated]("user.echo_reply", wskit.MessageOpts{Summary: "Echo reply"}),
			},
		}
		if adjust != nil {
			adjust(channel)
		}
		return wskit.NewModule(channel).ModuleDoc()
	}

	It("splits the channel into one operation per direction", func() {
		operations := msgdoc.DescribeChannel(wsChannel(nil), []string{"websocket"})

		Expect(operations).To(HaveLen(2))
		Expect(operations[0].Action).To(Equal(msgdoc.ActionReceive))
		Expect(operations[0].Messages[0].Name).To(Equal("user.echo"))
		Expect(operations[1].Action).To(Equal(msgdoc.ActionSend))
		Expect(operations[1].Messages[0].Name).To(Equal("user.echo_reply"))

		for _, op := range operations {
			Expect(op.Protocol).To(Equal(msgdoc.ProtocolWebSocket))
			Expect(op.Channel.Address).To(Equal("/ws/user"))
			Expect(op.Channel.ServerNames).To(Equal([]string{"websocket"}))
		}
	})

	It("records how a reader tells the messages on a channel apart", func() {
		operations := msgdoc.DescribeChannel(wsChannel(nil), nil)

		Expect(operations[0].Messages[0].When).To(Equal(`type is "user.echo"`))
	})

	It("omits a direction the channel never uses", func() {
		receiveOnly := wsChannel(func(c *wskit.Channel) { c.Send = nil })

		operations := msgdoc.DescribeChannel(receiveOnly, nil)

		// A send operation with an empty catalog would claim the server pushes
		// messages when it never does.
		Expect(operations).To(HaveLen(1))
		Expect(operations[0].Action).To(Equal(msgdoc.ActionReceive))
	})

	It("lifts path parameters out of the address", func() {
		parameterized := wsChannel(func(c *wskit.Channel) { c.Path = "/ws/rooms/{roomId}" })

		operations := msgdoc.DescribeChannel(parameterized, nil)

		Expect(operations[0].Channel.Parameters).To(HaveLen(1))
		Expect(operations[0].Channel.Parameters[0].Name).To(Equal("roomId"))
	})

	It("gives a WebSocket channel no delivery section", func() {
		operations := msgdoc.DescribeChannel(wsChannel(nil), nil)

		// A socket has no redelivery, acknowledgement or consumer group, so
		// there is nothing to describe rather than defaults to invent.
		Expect(operations[0].Behavior.Delivery).To(BeNil())
	})

	It("still lists a raw channel, marked untyped", func() {
		raw := &wskit.ModuleDoc{Path: "/ws/legacy", Untyped: true}

		operations := msgdoc.DescribeChannel(raw, nil)

		// A reachable channel missing from the document is worse than one the
		// reader can see is undocumented.
		Expect(operations).To(HaveLen(1))
		Expect(operations[0].Untyped).To(BeTrue())
		Expect(operations[0].Channel.Address).To(Equal("/ws/legacy"))
		Expect(operations[0].Messages).To(BeEmpty())
	})
})

var _ = Describe("message naming", func() {
	It("names a broker message after its Go type", func() {
		op := msgdoc.DescribeSubscription(subscriptionDoc(nil), "rabbitmq", nil)

		Expect(op.Messages[0].Name).To(Equal(reflect.TypeFor[userCreated]().Name()))
	})
})
