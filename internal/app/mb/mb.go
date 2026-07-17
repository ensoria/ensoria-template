package mb

import (
	"context"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/mb/pkg/mb"
)

// NewSubscribe は StartSubscription を生成する。
// サブスクリプションの生存期間は、アプリのルートコンテキスト（rootCtx）に一致させる。
// mb.StartSubscription が ctx を引数に取らないのは意図的で、生存期間 ctx として
// 正しい値は常にアプリのルート ctx ただ1つであり、これを呼び出し側に渡させると
// fx の OnStart フック ctx（起動完了時にキャンセルされる）を誤って渡す事故を招く。
// そのため、生存期間 ctx はこのアダプターが保持する。
func NewSubscribe(rootCtx dikit.RootContext, subConn mb.Subscriber) mb.StartSubscription {
	return func(target string, handler mb.SubscribeHandler, opts ...mb.SubscribeOption) error {
		// SubscribeHandlerのOnReceiveメソッドをMessageHandlerに変換
		// ctx はブローカー実装が受信時に供給する受信スコープのコンテキストで、OnReceiveへ伝播される
		messageHandler := func(ctx context.Context, data []byte, metadata map[string]string) error {
			return handler.OnReceive(ctx, data, metadata)
		}
		return subConn.Subscribe(rootCtx.Ctx, target, messageHandler, opts...)
	}
}

func NewPublish(pubConn mb.Publisher) mb.Publish {
	return func(ctx context.Context, target string, data []byte, metadata map[string]string, opts ...mb.PublishOption) error {
		// 呼び出し元（HTTP/gRPC等のコントローラー）のリクエストctxをそのまま伝播する
		return pubConn.Publish(ctx, target, data, metadata, opts...)
	}
}
