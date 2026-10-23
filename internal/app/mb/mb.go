package mb

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/mb/pkg/mb"
	"go.uber.org/fx"
)

// ErrReservedMetadata is wrapped by the error a publish returns when the
// application's own metadata names one of the headers who-started-this-work
// travels in (initiator.ReservedHeaders).
//
// Those are written from the context, by NewPublish, and by nothing else. A
// hand-written value beside them would send a different origin than the one
// the work has, without anything noticing.
var ErrReservedMetadata = errors.New("mb: metadata names a header the framework writes")

// LogTypeInitiatorNotCarried labels the startup record of a broker that cannot
// carry metadata, so who started the work is not sent with what is published.
const LogTypeInitiatorNotCarried = "mb_initiator_not_carried_log"

// NewSubscribe は StartSubscription を生成する。
// サブスクリプションの生存期間は、アプリのルートコンテキスト（rootCtx）に一致させる。
// mb.StartSubscription が ctx を引数に取らないのは意図的で、生存期間 ctx として
// 正しい値は常にアプリのルート ctx ただ1つであり、これを呼び出し側に渡させると
// fx の OnStart フック ctx（起動完了時にキャンセルされる）を誤って渡す事故を招く。
// そのため、生存期間 ctx はこのアダプターが保持する。
//
// Each message's metadata is read for who started the work it carries
// (initiator.FromHeaders, keeping only the claims in claims), and that is put
// on the handler's context, where initiator.From reads it. The reserved headers
// are then removed from the metadata the handler gets: reading them raw would
// skip the rules FromHeaders applies.
//
// The value is context, never a gate. A message without it reads as unknown,
// a malformed one as unknown with a warning, and either way the handler runs.
// A consumer has no Principal: nothing on this path was verified, and deciding
// what a party may do is the consumer's own data's business, keyed by the ID.
func NewSubscribe(rootCtx dikit.RootContext, subConn mb.Subscriber, claims *initiator.ClaimSet) mb.StartSubscription {
	return func(target string, handler mb.SubscribeHandler, opts ...mb.SubscribeOption) error {
		// SubscribeHandlerのOnReceiveメソッドをMessageHandlerに変換
		// ctx はブローカー実装が受信時に供給する受信スコープのコンテキストで、OnReceiveへ伝播される
		messageHandler := func(ctx context.Context, data []byte, metadata map[string]string) error {
			restored := initiator.FromHeaders(metadata, claims, target)
			return handler.OnReceive(initiator.With(ctx, restored), data, initiator.WithoutHeaders(metadata))
		}
		return subConn.Subscribe(rootCtx.Ctx, target, messageHandler, opts...)
	}
}

// NewPublish builds the publish function the application uses.
//
// It writes who started the work — initiator.From(ctx) — into the headers of
// every message, next to the application's own metadata, which it does not
// change. Metadata naming one of those headers is refused (ErrReservedMetadata).
//
// On a broker that cannot carry metadata (mb.Publisher.CarriesMetadata, which
// is false for Kinesis) the headers are left off, and that is said once, here,
// at startup: every publish being refused would cost far more than the audit
// trail that is lost. The reserved names are refused all the same, so code does
// not behave differently on one broker.
//
// ⚠ This is the one path that carries the value. Code that publishes through an
// mb.Publisher it was given directly sends no headers at all — another reason,
// besides the documentation, to publish through an mbkit.Publication.
func NewPublish(pubConn mb.Publisher) mb.Publish {
	carries := pubConn.CarriesMetadata()
	if !carries {
		loggear.Warn("the message broker cannot carry metadata, so who started the work is not sent with published messages",
			"type", LogTypeInitiatorNotCarried)
	}
	return func(ctx context.Context, target string, data []byte, metadata map[string]string, opts ...mb.PublishOption) error {
		if err := refuseReservedHeaders(metadata); err != nil {
			return fmt.Errorf("publishing to %s: %w", target, err)
		}
		if carries {
			metadata = withInitiatorHeaders(metadata, initiator.From(ctx))
		}
		// 呼び出し元（HTTP/gRPC等のコントローラー）のリクエストctxをそのまま伝播する
		return pubConn.Publish(ctx, target, data, metadata, opts...)
	}
}

// refuseReservedHeaders reports the reserved names metadata uses, if any.
func refuseReservedHeaders(metadata map[string]string) error {
	var named []string
	for key := range metadata {
		if initiator.IsReservedHeader(key) {
			named = append(named, key)
		}
	}
	if len(named) == 0 {
		return nil
	}
	slices.Sort(named)
	return fmt.Errorf("%w: %s (who started the work is written from the context; see initiator.From)",
		ErrReservedMetadata, strings.Join(named, ", "))
}

// withInitiatorHeaders returns a copy of metadata with the headers added. The
// caller's map is left alone: it may be reused for the next message.
func withInitiatorHeaders(metadata map[string]string, from *initiator.Initiator) map[string]string {
	merged := maps.Clone(metadata)
	if merged == nil {
		merged = make(map[string]string)
	}
	maps.Copy(merged, from.Headers())
	return merged
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
