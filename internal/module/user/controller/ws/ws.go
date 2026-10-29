package ws

import (
	"context"
	"fmt"

	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
)

// 接続開始時のハンドラ
type OnOpen struct {
	UserService service.UserService
}

func NewOnOpen(us service.UserService) *OnOpen {
	return &OnOpen{
		UserService: us,
	}
}

// The ctx passed to OnOpen is connection-scoped and carries the values set
// during the HTTP upgrade (e.g. by an auth HTTP middleware, see the wssession
// package docs). Thread it into the service / infra layer, deriving a
// per-operation context for outbound calls:
//
//	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
//	defer cancel()
//	result, err := c.SomeService.DoSomething(opCtx, ...)
//
// The ctx is canceled on server shutdown. NOTE: OnOpen runs before the read loop
// starts, so a disconnect during OnOpen is only observed after it returns; rely
// on the per-operation timeout above to bound slow OnOpen work.
func (c *OnOpen) OnOpen() wsconfig.OnOpenHandler {
	return func(ctx context.Context, event *wsevent.Open) error {
		fmt.Println("WebSocket connection opened")
		return nil
	}
}

func LogOnOpen(next wsconfig.OnOpenHandler) wsconfig.OnOpenHandler {
	return func(ctx context.Context, event *wsevent.Open) error {
		loggear.Info("WebSocket connection opened", "remote_addr", event.Conn.RemoteAddr().String())
		if next != nil {
			// Middlewares may enrich ctx (context.WithValue) before passing it on.
			return next(ctx, event)
		}
		return nil
	}
}

// maxLoggedMessageType bounds the message type a record repeats, in
// characters. The type is whatever the client wrote, and anyone may write to a
// public channel.
const maxLoggedMessageType = 64

// LogOnMessage records each message that arrives: which type it names, how
// large it is, and where it came from — never what it carries.
//
// A payload holds whatever a client typed, personal details or a token pasted
// by mistake included, and this runs before the message is validated or its
// sender authorized. The HTTP access record leaves request bodies out for the
// same reason.
func LogOnMessage(next wsconfig.OnMessageHandler) wsconfig.OnMessageHandler {
	return func(ctx context.Context, event *wsevent.Message) error {
		loggear.Info("WebSocket message received",
			"remote_addr", event.Conn.RemoteAddr().String(),
			"message_type", truncate(wskit.MessageTypeOf(event.MessageData), maxLoggedMessageType),
			"bytes", len(event.MessageData),
		)
		if next != nil {
			return next(ctx, event)
		}
		return nil
	}
}

// truncate shortens s to at most limit characters, never splitting one.
func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
