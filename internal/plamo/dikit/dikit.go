package dikit

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"google.golang.org/grpc"
)

const (
	GroupTagHttpModules     = `group:"http_modules"`
	GroupTagWSModules       = `group:"ws_modules"`
	GroupTagGRPCServices    = `group:"grpc_services"`
	GroupTagWorkerJobs      = `group:"worker_jobs"`
	GroupTagScheduledTasks  = `group:"scheduled_tasks"`
	GroupTagMBSubscriptions = `group:"mb_subscriptions"`
	GroupTagMBPublications  = `group:"mb_publications"`
)

// gRPCサービス登録用のインターフェース
type GRPCServiceRegistrar interface {
	RegisterWithServer(*grpc.Server)
}

type LC = fx.Lifecycle
type Hook = fx.Hook
type Shutdowner = fx.Shutdowner

var constructors = []any{}

// Constructorとして登録した関数は、参照されて初めて実行されます。
// 参照されていなくても、必ず実行してほしい関数は、AppendInvocationsを使って
// 登録してください。
// 登録するconstructor関数は、戻り値が必須です
func AppendConstructors(adding []any) {
	constructors = append(constructors, adding...)
}

func Constructors() []any {
	return constructors
}

var invocations = []any{}

// Invocationは、アプリ起動時に必ず実行されるものです。
// Constructorとは違い、参照されていなくても実行されます。
// 参照されていなくても必ず実行してほしい関数は、ここに登録してください。
// 登録するinvocation関数は戻り値は必須ではありません。
func AppendInvocations(adding []any) {
	invocations = append(invocations, adding...)
}

func Invocations() []any {
	return invocations
}

// === Providers ===

// Tのインターフェースに対して、該当するconcreteが1つだけの場合に使う
func ProvideAs[T any](concrete any) any {
	return fx.Annotate(concrete, fx.As(new(T)))
}

// 具象型をインターフェースTに変換して、名前付きで提供する
// 同じインターフェースに対して複数の実装がある場合に使用
//
// 使用例:
//
//	dikit.ProvideAsNamed[storage.Storage](storage.NewArchiveStorage, "ArchiveStorage")
//
// 注入側では `name:"ArchiveStorage"` タグで storage.Storage として受け取る
//
// ProvideNamedとの違い:
//   - ProvideAsNamed: 具象型 → インターフェースT に変換して名前付きで提供
//   - ProvideNamed: 具象型のまま名前付きで提供（型変換なし）
//
// 使い分け:
//   - 具象型をインターフェースとして抽象化したい場合 → ProvideAsNamed
//   - 具象型のまま、または既にインターフェース型を返す場合 → ProvideNamed
func ProvideAsNamed[T any](concrete any, tag string) any {
	return fx.Annotate(concrete, fx.As(new(T)), fx.ResultTags(`name:"`+tag+`"`))
}

// 具象型のまま名前付きで提供する（インターフェース変換なし）
// 同じ具象型を複数提供する場合や、インターフェースを使わない場合に使用
//
// 使用例:
//
//	dikit.ProvideNamed(grpcclt.NewPostConnection, grpcclt.PostConnName)
//
// 注入側では `name:"PostConn"` タグで grpc.ClientConnInterface として受け取る
//
// ProvideAsNamedとの違い:
//   - ProvideAsNamed: 具象型 → インターフェースT に変換して名前付きで提供
//   - ProvideNamed: 具象型のまま名前付きで提供（型変換なし）
//
// 使い分け:
//   - インターフェースとして抽象化したい場合 → ProvideAsNamed
//   - 具象型のまま提供したい場合（grpc.ClientConnInterfaceなど既にインターフェースの場合）→ ProvideNamed
func ProvideNamed(constructor any, tag string) any {
	return fx.Annotate(constructor, fx.ResultTags(`name:"`+tag+`"`))
}

func AsHTTPModule(f any) any {
	return fx.Annotate(
		f,
		fx.ResultTags(GroupTagHttpModules),
	)
}

func AsWSModule(f any) any {
	return fx.Annotate(
		f,
		fx.ResultTags(GroupTagWSModules),
	)
}

func AsGRPCService(f any) any {
	return fx.Annotate(
		f,
		fx.As(new(GRPCServiceRegistrar)),
		fx.ResultTags(GroupTagGRPCServices),
	)
}

func AsWorkerJob(f any) any {
	return fx.Annotate(
		f,
		fx.ResultTags(GroupTagWorkerJobs),
	)
}

func AsScheduledTask(f any) any {
	return fx.Annotate(
		f,
		fx.ResultTags(GroupTagScheduledTasks),
	)
}

// メッセージブローカーの購読宣言(*mbkit.SubscriptionModule)をgroupに登録する。
//
// 以前のように起動用のinvocationをモジュールごとに書く必要はない。app層の
// invocationが1つ、このgroupを走査してまとめて購読を開始する。
// group経由にすることで、購読対象とオプションが宣言として残り、
// describeがリフレクションで読み取れるようになる。
func AsMBSubscription(f any) any {
	return fx.Annotate(
		f,
		fx.ResultTags(GroupTagMBSubscriptions),
	)
}

// メッセージブローカーの発行宣言をドキュメント用groupに登録する。
//
// 登録するのは`*mbkit.Publication[Msg]`本体ではなく、
// `mbkit.AsPublicationDoc[Msg]`のような非ジェネリックなインターフェースへの
// アダプタである。serviceには`*mbkit.Publication[Msg]`を型付きのまま注入し、
// describeは同じ宣言オブジェクトをこのgroupから見る。
//
// 使用例:
//
//	dikit.AsMBPublication(mbkit.AsPublicationDoc[dto.UserCreated]),
func AsMBPublication(f any) any {
	return fx.Annotate(
		f,
		fx.ResultTags(GroupTagMBPublications),
	)
}

// === Injectors ===

// 汎用版 - 複数の引数位置に対してタグを指定可能
// 例:
// dikit.InjectWithTags(SomeConstructor, `name:"Something"`, `group:"items"`),
func InjectWithTags(constructor any, tags ...string) any {
	return fx.Annotate(constructor, fx.ParamTags(tags...))
}

// gRPCクライアントの注入用
// 実際には引数が1つだけの場合は汎用的に使えますが、
// 汎用的に使いたい場合は、別の関数を用意するか、IbjectWithTagsを使ってください。
func InjectGRPCClient(constructor any, tag string) any {
	return fx.Annotate(constructor, fx.ParamTags(`name:"`+tag+`"`))
}

// === Lifecycles ===

func RegisterLifecycle(lc LC, onStart func(ctx context.Context) error, onStop func(ctx context.Context) error) {
	lc.Append(Hook{
		OnStart: onStart,
		OnStop:  onStop,
	})
}

func RegisterOnStartLifecycle(lc LC, onStart func(ctx context.Context) error) {
	lc.Append(Hook{
		OnStart: onStart,
	})
}

func RegisterOnStopLifecycle(lc LC, onStop func(ctx context.Context) error) {
	lc.Append(Hook{
		OnStop: onStop,
	})
}

// === Root Context ===

// RootContext はアプリケーションの生存期間に一致するルートコンテキスト。
// メッセージブローカーのSubscriberなど、アプリが動いている間ずっと動作し続ける
// 常駐処理は、この Ctx を生存期間として使う。
//
// IMPORTANT: fx の OnStart / OnStop フックが受け取る ctx は、その起動・停止処理
// のためのタイムアウト付き ctx であり、起動完了時（または停止完了時）にキャンセル
// される。常駐処理の生存期間には使ってはならない。
type RootContext struct {
	Ctx context.Context
}

// ProvideRootContext はアプリのルートコンテキストを生成し、OnStop でキャンセル
// されるように登録する。fx のコンストラクタとして登録して使う。
func ProvideRootContext(lc LC) RootContext {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(Hook{
		OnStop: func(context.Context) error {
			// シャットダウン時にルートコンテキストをキャンセルし、常駐処理に停止を通知する
			cancel()
			return nil
		},
	})
	return RootContext{Ctx: ctx}
}

// === Fx App Run ===

// ExitError reports that the application asked to terminate with a non-zero
// exit code via Shutdowner.Shutdown(ExitCode(n)). main() exits with Code and
// logs it at info level; the error-level record that says why belongs to
// whoever requested the code.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("application requested exit code %d", e.Code)
}

// ProvideAndRun builds the fx application, starts it, waits for a shutdown
// signal and stops it, reporting every failure to the caller.
//
// It deliberately does not use fx.App.Run(). Run() writes its failures to the
// event logger only, and the logger is NopLogger whenever fx's verbose output
// is off — which is every environment but a debug one. A startup failure would
// then end the process without a single line explaining why. Returning the
// error instead puts every failure on one path, in one record shape, written
// by whoever called this.
//
// The observable behaviour of Run() is otherwise reproduced: Start, Wait and
// Stop are the pieces Run() itself is made of, signal handling lives in Wait,
// and the Stopping event is emitted at the same point Run() emits it.
func ProvideAndRun(constructors []any, invocations []any, outputFxLog bool) error {
	// The event logger is NopLogger when quiet. When verbose it is the very
	// logger fx installs by default; it is passed explicitly only so that we
	// hold a reference to it, since fx.App does not expose its logger and the
	// Stopping event below has to reach the same destination.
	logger := fxevent.Logger(fxevent.NopLogger)
	if outputFxLog {
		logger = &fxevent.ConsoleLogger{W: os.Stderr}
	}

	app := fx.New(
		fx.Provide(
			constructors...,
		),
		fx.Invoke(invocations...),
		fx.WithLogger(func() fxevent.Logger { return logger }),
	)

	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()
	// Start reports both kinds of failure: a constructor or an invocation that
	// failed while the graph was built, and an OnStart hook that failed.
	// Stop must not be called on this path — fx already rolled back the hooks
	// that had started and merged the rollback errors into this one.
	if err := app.Start(startCtx); err != nil {
		return err
	}

	sig := <-app.Wait()
	logger.LogEvent(&fxevent.Stopping{Signal: sig.Signal})

	stopCtx, stopCancel := context.WithTimeout(context.Background(), app.StopTimeout())
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		// Matches fx's own Run(): a failed shutdown outranks a requested exit
		// code.
		return fmt.Errorf("shutdown failed: %w", err)
	}

	if sig.ExitCode != 0 {
		return &ExitError{Code: sig.ExitCode}
	}
	return nil
}
