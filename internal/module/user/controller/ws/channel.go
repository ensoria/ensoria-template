package ws

import (
	"context"
	"time"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
)

// Message names carried on this channel. Declaring them as constants keeps the
// receive side, the send side and the client from drifting onto names that
// merely look alike.
const (
	EchoMessage      = "user.echo"
	EchoReplyMessage = "user.echo_reply"
)

// EchoReply is the sender for the reply the server pushes back.
//
// It is a package-level value because the same declaration is used twice: the
// handler sends through it, and the channel registers it in its Send catalog.
// One value means the documented message and the sent message cannot diverge.
var EchoReply = wskit.Send[dto.UserEchoReply](EchoReplyMessage, wskit.MessageOpts{
	Summary:     "Echo reply pushed back to the client",
	Description: "Sent once for each user.echo received.",
	FieldDocs: map[string]string{
		"message":     "the text that was echoed back",
		"received_at": "server time the original message was handled",
	},
})

// Channel is the typed WebSocket channel of the user module.
type Channel struct {
	UserService service.UserService
	OnOpen      *OnOpen
}

func NewChannel(us service.UserService, onOpen *OnOpen) *Channel {
	return &Channel{UserService: us, OnOpen: onOpen}
}

// Declare builds the channel declaration: the path, the message catalog and the
// lifecycle handlers.
func (c *Channel) Declare() *wskit.Channel {
	return &wskit.Channel{
		Path:        "/ws/user",
		Summary:     "User real-time channel",
		Description: "Demonstrates the typed message catalog: the client sends user.echo and the server answers with user.echo_reply.",
		Task:        "demo realtime",
		Behavior: wskit.BehaviorSpec{
			SideEffects:   []string{"none"},
			Idempotent:    new(true),
			Preconditions: []string{"none"},
			Ordering:      "messages are ordered within one connection, not across reconnects",
		},
		// Set all `OnMessage` handlers here, one per message type.
		// sending data must be JSON and must have `type` and `data` fields.
		// The `type` field must be string and must match `wskit.Receive[T]()` first argument string
		// The `data` field must be a JSON object
		Receive: []*wskit.Receiver{
			wskit.Receive[dto.UserEcho](EchoMessage, wskit.MessageOpts{
				Summary: "Echo request sent by the client",
				FieldDocs: map[string]string{
					"message": "text the client wants echoed back",
				},
				BodyRules: []*rule.RuleSet{
					{Field: "message", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(200)}},
				},
			}, c.handleEcho),
		},
		Send: []wskit.DocumentedMessage{EchoReply},
		// Any method except `OnMessage` can be set here, including middlewares.
		Configure: func(m *wsconfig.Module) {
			// for logging
			m.AddOnOpenMiddleware(LogOnOpen)
			m.OnOpen = c.OnOpen.OnOpen()
			// for logging
			m.AddOnMessageMiddleware(LogOnMessage)
		},
	}
}

// handleEcho receives the decoded, validated message and answers through the
// declared sender.
func (c *Channel) handleEcho(ctx context.Context, event *wsevent.Message, msg *dto.UserEcho) error {
	return EchoReply.Send(ctx, event.Conn, &dto.UserEchoReply{
		Message:    msg.Message,
		ReceivedAt: time.Now().UTC(),
	})
}
