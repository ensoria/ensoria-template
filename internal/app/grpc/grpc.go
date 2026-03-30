package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/grpcgear/pkg/interceptor/logging/logsrv"
	"github.com/ensoria/grpcgear/pkg/interceptor/recovery/recoverysrv"
	"github.com/ensoria/loggear/pkg/loggear"
	"go.uber.org/fx"
	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// gRPCサーバーの初期化
func NewGRPCApp(envVal *string) func(lc dikit.LC, shutdowner dikit.Shutdowner, grpcServices []dikit.GRPCServiceRegistrar) *ggrpc.Server {
	return func(lc dikit.LC, shutdowner dikit.Shutdowner, grpcServices []dikit.GRPCServiceRegistrar) *ggrpc.Server {
		// ログとpanicリカバリinterceptor付きのgRPCサーバーを作成
		grpcSrv := NewGRPCServer(loggear.GetLogger())

		// reflectionは開発環境でのみ有効にする
		// TODO: config/env にIsLocal()を作って、それを使う
		if envVal != nil && (*envVal == string(env.Local) || *envVal == string(env.Develop)) {
			reflection.Register(grpcSrv)
			loggear.Info("gRPC reflection enabled for development environment", "env", *envVal)
		}

		RegisterGRPCServerLifecycle(lc, shutdowner, grpcSrv)

		for _, svc := range grpcServices {
			svc.RegisterWithServer(grpcSrv)
		}
		loggear.Info("gRPC services registered", "count", len(grpcServices))

		return grpcSrv
	}
}

func NewGRPCServer(logger loggear.Logger) *ggrpc.Server {
	logCfg := LogConfig()
	recCfg := recoverysrv.DefaultRecoveryConfig()
	logUnarySuccess, logUnaryError := CreateBasicUnaryLogFuncs(logger)
	logStreamSuccess, logStreamError := CreateBasicStreamLogFuncs(logger)
	logUnaryPanic, logStreamPanic := CreateBasicPanicLogFuncs(logger)

	// チェーン化された複数のinterceptorを作成
	// 注意: 実行される順番は引数で渡す順番です。
	// そのため、確実にpanicを拾う場合はrecoveryを最初に配置すべきです
	opts := []ggrpc.ServerOption{
		ggrpc.ChainUnaryInterceptor(
			recoverysrv.RecoveryUnaryInterceptor(logUnaryPanic, logCfg, recCfg), // 最外側: panic を最初にキャッチ
			logsrv.LoggingUnaryInterceptor(logUnarySuccess, logUnaryError, logCfg),
		),
		ggrpc.ChainStreamInterceptor(
			recoverysrv.RecoveryStreamInterceptor(logStreamPanic, logCfg, recCfg), // 最外側: panic を最初にキャッチ
			logsrv.LoggingStreamInterceptor(logStreamSuccess, logStreamError, logCfg),
		),
	}

	return ggrpc.NewServer(opts...)
}

// gRPC server lifecycle registration
func RegisterGRPCServerLifecycle(lc dikit.LC, shutdowner dikit.Shutdowner, grpcSrv *ggrpc.Server) {
	if grpcSrv == nil {
		return
	}

	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			// TODO: ポートを設定可能にする
			listen, err := net.Listen("tcp", ":50051")
			if err != nil {
				return fmt.Errorf("gRPC server failed to listen: %w", err)
			}
			go func() {
				loggear.Info("gRPC server starting", "addr", ":50051")
				if err := grpcSrv.Serve(listen); err != nil {
					loggear.Error("gRPC server stopped unexpectedly", "error", err)
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			loggear.Info("Shutting down gRPC server")
			done := make(chan struct{})
			go func() {
				grpcSrv.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				grpcSrv.Stop() // 強制停止
				return nil
			}
		},
	})
}

func InjectGRPCServices(f any) any {
	return fx.Annotate(
		f,
		fx.ParamTags(``, ``, dikit.GroupTagGRPCServices),
	)
}
