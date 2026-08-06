package mb

import (
	"context"
	"fmt"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/mb/pkg/mb"
	"go.uber.org/fx"
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

// StartSubscriptions は宣言された購読(mb_subscriptions group)をまとめて開始する。
//
// モジュールごとに起動用のinvocationを書く方式をこれに置き換えている。購読対象と
// オプションが起動クロージャの中に埋もれていると、リフレクションで読み取れず
// ドキュメントを生成できない。宣言をgroupに集めることで、起動経路と
// describeが同じ宣言を見るようになる。
//
// NOTE: OnStartのctxはfxの起動処理用ctx（起動完了時にキャンセルされる）なので、
// 購読の生存期間には使わない。生存期間ctxはNewSubscribe（StartSubscription生成側）が
// アプリのルートコンテキストとして保持する。そのためsubscribeはctxを引数に取らない。
func StartSubscriptions(lc dikit.LC, subscribe mb.StartSubscription, modules []*mbkit.SubscriptionModule) {
	onStart := func(context.Context) error {
		for _, m := range modules {
			loggear.Info("start subscribing", "target", m.Target())
			if err := subscribe(m.Target(), m, m.Options()...); err != nil {
				return fmt.Errorf("subscribing to %s: %w", m.Target(), err)
			}
		}
		return nil
	}
	dikit.RegisterOnStartLifecycle(lc, onStart)
}

// InjectMBSubscriptions は3番目の引数を購読宣言のgroupとしてタグ付けする。
// 残りの引数（ライフサイクル、StartSubscription）は型で解決される。
func InjectMBSubscriptions(f any) any {
	return fx.Annotate(f, fx.ParamTags(``, ``, dikit.GroupTagMBSubscriptions))
}
