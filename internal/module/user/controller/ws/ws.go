package ws

import (
	"context"
	"fmt"

	"github.com/ensoria/ensoria-template/internal/module/user/service"
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

func LogOnMessage(next wsconfig.OnMessageHandler) wsconfig.OnMessageHandler {
	return func(ctx context.Context, event *wsevent.Message) error {
		loggear.Info("WebSocket message received", "remote_addr", event.Conn.RemoteAddr().String(), "message", string(event.MessageData))
		if next != nil {
			return next(ctx, event)
		}
		return nil
	}
}
