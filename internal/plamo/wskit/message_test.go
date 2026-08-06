package wskit_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/websocket/pkg/wsevent"
)

var _ = Describe("Sender", func() {
	// The envelope a Sender writes is asserted end-to-end in the dispatch specs,
	// which read it off a real connection. These specs cover what happens before
	// anything reaches the wire.

	It("refuses to send a payload that breaks its declared rules", func() {
		sender := wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{
			BodyRules: []*rule.RuleSet{
				{Field: "message", Rules: []rule.Rule{vkit.Required()}},
			},
		})

		err := sender.Send(context.Background(), nil, &echo{Message: ""})

		// Sending a message that violates the contract this declaration
		// documents is worth stopping here, the last place it still can be.
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("test.echo_reply"))
		Expect(err.Error()).To(ContainSubstring("message"))
	})

	It("does not write once the connection context is canceled", func() {
		sender := wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := sender.Send(ctx, nil, &echo{Message: "hi"})

		Expect(err).To(MatchError(context.Canceled))
	})

	It("applies the same checks to a broadcast", func() {
		sender := wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{
			BodyRules: []*rule.RuleSet{
				{Field: "message", Rules: []rule.Rule{vkit.Required()}},
			},
		})

		Expect(sender.Broadcast(context.Background(), nil, &echo{})).To(HaveOccurred())
	})

	It("reports the name it was declared with", func() {
		Expect(wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{}).Name()).
			To(Equal("test.echo_reply"))
	})
})

var _ = Describe("MessageDoc", func() {
	It("carries a sender's declaration", func() {
		sender := wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{
			Summary:     "Echo reply",
			Description: "Longer explanation",
			FieldDocs:   map[string]string{"message": "the echoed text"},
		})

		doc := sender.MessageDoc()

		Expect(doc.Name).To(Equal("test.echo_reply"))
		Expect(doc.Summary).To(Equal("Echo reply"))
		Expect(doc.Description).To(Equal("Longer explanation"))
		Expect(doc.FieldDocs).To(HaveKeyWithValue("message", "the echoed text"))
		Expect(doc.MsgType).To(Equal(reflect.TypeFor[echo]()))
	})

	It("carries a receiver's declaration, including its rules", func() {
		rules := []*rule.RuleSet{
			{Field: "message", Rules: []rule.Rule{vkit.Required()}},
		}
		receiver := wskit.Receive[echo]("test.echo", wskit.MessageOpts{
			Summary:   "Echo request",
			BodyRules: rules,
		}, func(context.Context, *wsevent.Message, *echo) error { return nil })

		doc := receiver.MessageDoc()

		Expect(doc.Name).To(Equal("test.echo"))
		Expect(doc.Summary).To(Equal("Echo request"))
		Expect(doc.MsgType).To(Equal(reflect.TypeFor[echo]()))
		Expect(doc.BodyRules).To(HaveLen(1))
	})

	It("explains the discriminator by default, since that is what tells messages apart", func() {
		doc := wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{}).MessageDoc()

		Expect(doc.When).To(Equal(`type is "test.echo_reply"`))
	})

	It("keeps a declared When, for a message the envelope alone does not identify", func() {
		doc := wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{
			When: "the echoed text was truncated",
		}).MessageDoc()

		Expect(doc.When).To(Equal("the echoed text was truncated"))
	})
})
