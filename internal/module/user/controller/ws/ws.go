package ws

import (
	"fmt"

	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/ensoria/websocket/pkg/wssend"
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

func (c *OnOpen) OnOpen() wsconfig.OnOpenHandler {
	return func(event *wsevent.Open) error {
		fmt.Println("WebSocket connection opened")
		return nil
	}
}

func LogOnOpen(next wsconfig.OnOpenHandler) wsconfig.OnOpenHandler {
	return func(event *wsevent.Open) error {
		loggear.Info("WebSocket connection opened", "remote_addr", event.Conn.RemoteAddr().String())
		if next != nil {
			return next(event)
		}
		return nil
	}
}

// メッセージ受信時のハンドラ
type OnMessage struct {
	UserService service.UserService
}

func NewOnMessage(us service.UserService) *OnMessage {
	return &OnMessage{
		UserService: us,
	}
}

func (c *OnMessage) OnMessage() wsconfig.OnMessageHandler {
	return func(event *wsevent.Message) error {
		wssend.Text(event.Conn, "Hello from server. Received your message: "+string(event.MessageData))
		return nil
	}
}

func LogOnMessage(next wsconfig.OnMessageHandler) wsconfig.OnMessageHandler {
	return func(event *wsevent.Message) error {
		loggear.Info("WebSocket message received", "remote_addr", event.Conn.RemoteAddr().String(), "message", string(event.MessageData))
		if next != nil {
			return next(event)
		}
		return nil
	}
}
