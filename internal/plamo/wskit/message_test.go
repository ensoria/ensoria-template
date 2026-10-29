package wskit_test

import (
	"context"
	"fmt"
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
		sender := wskit.Send[echo]("test.echo_reply", wskit.SendOpts{
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
		sender := wskit.Send[echo]("test.echo_reply", wskit.SendOpts{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := sender.Send(ctx, nil, &echo{Message: "hi"})

		Expect(err).To(MatchError(context.Canceled))
	})

	It("applies the same checks to a broadcast", func() {
		sender := wskit.Send[echo]("test.echo_reply", wskit.SendOpts{
			BodyRules: []*rule.RuleSet{
				{Field: "message", Rules: []rule.Rule{vkit.Required()}},
			},
		})

		Expect(sender.Broadcast(context.Background(), nil, &echo{})).To(HaveOccurred())
	})

	It("reports the name it was declared with", func() {
		Expect(wskit.Send[echo]("test.echo_reply", wskit.SendOpts{}).Name()).
			To(Equal("test.echo_reply"))
	})
})

var _ = Describe("MessageDoc", func() {
	It("carries a sender's declaration", func() {
		sender := wskit.Send[echo]("test.echo_reply", wskit.SendOpts{
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
		receiver := wskit.Receive[echo]("test.echo", wskit.ReceiveOpts{
			Summary:   "Echo request",
			BodyRules: rules,
		}, func(context.Context, *wsevent.Message, *echo) error { return nil })

		doc := receiver.MessageDoc()

		Expect(doc.Name).To(Equal("test.echo"))
		Expect(doc.Summary).To(Equal("Echo request"))
		Expect(doc.MsgType).To(Equal(reflect.TypeFor[echo]()))
		Expect(doc.BodyRules).To(HaveLen(1))
	})

	It("carries a receiver's security declaration", func() {
		security := &wskit.MessageSecurity{Scopes: []string{"things:write"}}
		receiver := wskit.Receive[echo]("test.echo", wskit.ReceiveOpts{Security: security},
			func(context.Context, *wsevent.Message, *echo) error { return nil })

		Expect(receiver.Security()).To(BeIdenticalTo(security))
		Expect(receiver.MessageDoc().Security).To(BeIdenticalTo(security))
	})

	It("carries no security declaration for a sender, which has none to make", func() {
		Expect(wskit.Send[echo]("test.echo_reply", wskit.SendOpts{}).MessageDoc().Security).To(BeNil())
	})

	It("explains the discriminator by default, since that is what tells messages apart", func() {
		doc := wskit.Send[echo]("test.echo_reply", wskit.SendOpts{}).MessageDoc()

		Expect(doc.When).To(Equal(`type is "test.echo_reply"`))
	})

	It("keeps a declared When, for a message the envelope alone does not identify", func() {
		doc := wskit.Send[echo]("test.echo_reply", wskit.SendOpts{
			When: "the echoed text was truncated",
		}).MessageDoc()

		Expect(doc.When).To(Equal("the echoed text was truncated"))
	})
})

var _ = Describe("ReceiveOpts and SendOpts", func() {
	fieldsOf := func(t reflect.Type) []string {
		fields := make([]string, 0, t.NumField())
		for f := range t.Fields() {
			fields = append(fields, fmt.Sprintf("%s %s", f.Name, f.Type))
		}
		return fields
	}

	// Both directions describe a message the same way, so a field added to one
	// and forgotten on the other would make the documentation of the two
	// directions drift apart without anything failing.
	It("share every field but Security, with the same types in the same order", func() {
		receive := fieldsOf(reflect.TypeFor[wskit.ReceiveOpts]())
		withoutSecurity := make([]string, 0, len(receive))
		for _, f := range receive {
			if f != fmt.Sprintf("Security %s", reflect.TypeFor[*wskit.MessageSecurity]()) {
				withoutSecurity = append(withoutSecurity, f)
			}
		}

		Expect(fieldsOf(reflect.TypeFor[wskit.SendOpts]())).To(Equal(withoutSecurity))
	})

	// A condition written on a sent message would read as "only these callers
	// receive it" while nothing enforced it: everyone would. Not having the
	// field makes writing it a compile error instead.
	It("give Security to a received message only", func() {
		_, onReceive := reflect.TypeFor[wskit.ReceiveOpts]().FieldByName("Security")
		_, onSend := reflect.TypeFor[wskit.SendOpts]().FieldByName("Security")

		Expect(onReceive).To(BeTrue())
		Expect(onSend).To(BeFalse())
	})
})
