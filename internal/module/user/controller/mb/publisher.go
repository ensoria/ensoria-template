package mb

import (
	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	enmb "github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
)

// UserCreatedTarget is the channel the user-created event is announced on.
const UserCreatedTarget = "user.created"

// NewHelloWorldPublication declares the publication side of the hello_world
// channel, which this application also subscribes to.
//
// Publishing through the declaration is what puts the message in the generated
// document: a raw mb.Publish call would emit a message no document knows about.
func NewHelloWorldPublication(publish enmb.Publish) *mbkit.Publication[dto.HelloWorld] {
	return mbkit.NewPublication(publish, &mbkit.PublicationSpec[dto.HelloWorld]{
		Target:      HelloWorldTarget,
		Summary:     "Emit the demonstration hello_world message",
		Description: "Sample publication paired with the hello_world subscription, so the same channel shows both directions.",
		Task:        "demo send",
		FieldDocs: map[string]string{
			"message": "free-form text carried by the message",
			"source":  "name of the code path that published it",
		},
		BodyRules: []*rule.RuleSet{
			{Field: "message", Rules: []rule.Rule{vkit.Required()}},
		},
		Behavior: mbkit.BehaviorSpec{
			SideEffects:   []string{"none"},
			Idempotent:    ptr(true),
			Preconditions: []string{"none"},
			Ordering:      "none",
			Delivery: mbkit.DeliverySpec{
				Guarantee: "at-most-once",
			},
		},
	})
}

// NewUserCreatedPublication declares the user-created event.
//
// Unlike hello_world, this one is meant for consumers outside this application.
// That is precisely why it is declared: in pub/sub the publisher owns the
// contract, so this declaration is the only place the shape of the event, its
// delivery guarantee and its ordering can be written down for the teams that
// subscribe to it.
func NewUserCreatedPublication(publish enmb.Publish) *mbkit.Publication[dto.UserCreated] {
	return mbkit.NewPublication(publish, &mbkit.PublicationSpec[dto.UserCreated]{
		Target:      UserCreatedTarget,
		Summary:     "Announce that a user was created",
		Description: "Emitted once the user has been persisted. Consumers may treat it as the authoritative record that the user now exists.",
		Task:        "announce user",
		FieldDocs: map[string]string{
			"id":    "identifier of the created user",
			"name":  "display name given at creation",
			"email": "contact address, omitted when the user supplied none",
		},
		BodyRules: []*rule.RuleSet{
			{Field: "id", Rules: []rule.Rule{vkit.NumNotZero()}},
			{Field: "name", Rules: []rule.Rule{vkit.Required()}},
		},
		Behavior: mbkit.BehaviorSpec{
			SideEffects: []string{"none in this application; consumers act on it"},
			// Consumers may see the event more than once, so their handling has
			// to be idempotent even though emitting it is not repeated.
			Idempotent:    ptr(true),
			Preconditions: []string{"the user has been persisted"},
			Ordering:      "none; consumers must not assume creation order across users",
			Delivery: mbkit.DeliverySpec{
				Guarantee: "at-least-once",
				Notes: []string{
					"Published with persistent delivery, so a broker restart does not lose the event.",
				},
			},
		},
		Options: []enmb.PublishOption{
			enmb.WithPersistentDelivery(),
		},
	})
}
