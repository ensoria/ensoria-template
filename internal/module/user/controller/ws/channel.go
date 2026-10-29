package ws

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
)

// Message names carried on this channel. Declaring them as constants keeps the
// receive side, the send side and the client from drifting onto names that
// merely look alike.
const (
	EchoMessage         = "user.echo"
	EchoReplyMessage    = "user.echo_reply"
	WhoAmIMessage       = "user.whoami"
	WhoAmIReplyMessage  = "user.whoami_reply"
	AnnounceMessage     = "user.announce"
	AnnouncementMessage = "user.announcement"
)

// channelDescription is the longer explanation of the channel.
const channelDescription = "Demonstrates the typed message catalog: the client sends user.echo and the server " +
	"answers with user.echo_reply. Anyone may connect and echo; user.whoami needs a signed-in caller, " +
	"and user.announce a caller holding announcements:write."

// announceScope is what a caller needs to send an announcement.
const announceScope = "announcements:write"

// LogTypeAnnouncementUndelivered labels an announcement that did not reach
// every connection on the channel, because some of them were going away.
const LogTypeAnnouncementUndelivered = "user_announcement_undelivered_log"

// EchoReply is the sender for the reply the server pushes back.
//
// It is a package-level value because the same declaration is used twice: the
// handler sends through it, and the channel registers it in its Send catalog.
// One value means the documented message and the sent message cannot diverge.
var EchoReply = wskit.Send[dto.UserEchoReply](EchoReplyMessage, wskit.SendOpts{
	Summary:     "Echo reply pushed back to the client",
	Description: "Sent once for each user.echo received.",
	FieldDocs: map[string]string{
		"message":     "the text that was echoed back",
		"received_at": "server time the original message was handled",
	},
})

// WhoAmIReply is the sender for the answer to user.whoami.
var WhoAmIReply = wskit.Send[dto.UserWhoAmIReply](WhoAmIReplyMessage, wskit.SendOpts{
	Summary:     "Who the connection belongs to",
	Description: "Sent once for each user.whoami received.",
	FieldDocs: map[string]string{
		"subject": "the caller's identifier",
		"scheme":  "the kind of credential the connection was opened with (jwt, apiKey or session)",
	},
})

// Announcement is the sender for an announcement, pushed to every connection
// on the channel.
var Announcement = wskit.Send[dto.UserAnnouncement](AnnouncementMessage, wskit.SendOpts{
	Summary:     "An announcement, pushed to everyone on the channel",
	Description: "Sent to every connection on /ws/user, anonymous ones included, for each user.announce accepted.",
	FieldDocs: map[string]string{
		"message": "the announced text",
		"sent_at": "server time the announcement was accepted",
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
		Description: channelDescription,
		Task:        "demo realtime",
		// Anyone may connect, anonymous callers included: this is the echo
		// demonstration. A channel that declares nothing requires a verified
		// caller, so opening one has to be written down, as it is here.
		Security: &wskit.SecuritySpec{Public: true},
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
			wskit.Receive[dto.UserEcho](EchoMessage, wskit.ReceiveOpts{
				Summary: "Echo request sent by the client",
				FieldDocs: map[string]string{
					"message": "text the client wants echoed back",
				},
				BodyRules: []*rule.RuleSet{
					{Field: "message", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(200)}},
				},
			}, c.handleEcho),
			// A message can ask more of its sender than the channel asks of a
			// connection. An empty declaration reads "signed in": an anonymous
			// connection is answered with unauthenticated and stays open.
			wskit.Receive[dto.UserWhoAmI](WhoAmIMessage, wskit.ReceiveOpts{
				Summary:  "Ask who the connection belongs to",
				Security: &wskit.MessageSecurity{},
			}, c.handleWhoAmI),
			// Anyone connected hears an announcement; only a caller holding
			// announcements:write may make one. A scope the application's
			// policy lets stand for it counts too, so admin may announce as well.
			wskit.Receive[dto.UserAnnounce](AnnounceMessage, wskit.ReceiveOpts{
				Summary: "Announce something to everyone on the channel",
				FieldDocs: map[string]string{
					"message": "the text to announce",
				},
				BodyRules: []*rule.RuleSet{
					{Field: "message", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(200)}},
				},
				Security: &wskit.MessageSecurity{Scopes: []string{announceScope}},
			}, c.handleAnnounce),
		},
		Send: []wskit.DocumentedMessage{EchoReply, WhoAmIReply, Announcement},
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

// handleWhoAmI answers with the caller the connection belongs to, read from ctx
// as it is everywhere else.
func (c *Channel) handleWhoAmI(ctx context.Context, event *wsevent.Message, _ *dto.UserWhoAmI) error {
	caller, ok := authkit.PrincipalFrom(ctx)
	if !ok {
		// The declaration admits only a verified caller, so this is reached only
		// if it is removed. Answering for nobody would hide that.
		return errors.New("user.whoami reached its handler without a caller")
	}
	return WhoAmIReply.Send(ctx, event.Conn, &dto.UserWhoAmIReply{
		Subject: caller.Subject,
		Scheme:  caller.Scheme,
	})
}

// handleAnnounce passes the announcement on to every connection on this path
// on this instance, the announcer's own included.
//
// A connection that could not be written to is logged rather than returned: it
// is that connection which is going away, and returning the error would close
// the announcer's instead.
func (c *Channel) handleAnnounce(ctx context.Context, event *wsevent.Message, msg *dto.UserAnnounce) error {
	err := Announcement.Broadcast(ctx, event.Session.Connections(), &dto.UserAnnouncement{
		Message: msg.Message,
		SentAt:  time.Now().UTC(),
	})
	if err != nil {
		loggear.Warn("an announcement did not reach every connection on the channel",
			slog.String("type", LogTypeAnnouncementUndelivered),
			slog.String("error", err.Error()),
		)
	}
	return nil
}
