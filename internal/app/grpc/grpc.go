package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/grpcgear/pkg/interceptor/logging/logsrv"
	"github.com/ensoria/grpcgear/pkg/interceptor/recovery/recoverysrv"
	"github.com/ensoria/loggear/pkg/loggear"
	"go.uber.org/fx"
	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// defaultModule is the configuration module the gRPC settings are read from.
const defaultModule = "default"

// gRPCサーバーの初期化
//
// A failure is returned rather than written with log.Fatal, the same as the
// HTTP app: fx reports it through the one path every other startup failure
// takes.
func NewGRPCApp(envVal *string) func(lc dikit.LC, shutdowner dikit.Shutdowner, grpcServices []dikit.GRPCServiceRegistrar) (*ggrpc.Server, error) {
	return func(lc dikit.LC, shutdowner dikit.Shutdowner, grpcServices []dikit.GRPCServiceRegistrar) (*ggrpc.Server, error) {
		params, err := registry.ModuleParams(defaultModule)
		if err != nil {
			return nil, fmt.Errorf("default config parameters not found: %w", err)
		}
		config := params.GRPC

		// ログとpanicリカバリinterceptor付きのgRPCサーバーを作成
		grpcSrv := NewGRPCServer(loggear.GetLogger(), config)

		if config.ReflectionEnabled(reflectionDefaultForEnv(envVal)) {
			reflection.Register(grpcSrv)
			loggear.Info("gRPC reflection enabled", "env", envName(envVal))
		}

		RegisterGRPCServerLifecycle(lc, shutdowner, grpcSrv, config)

		for _, svc := range grpcServices {
			svc.RegisterWithServer(grpcSrv)
		}
		loggear.Info("gRPC services registered", "count", len(grpcServices))

		return grpcSrv, nil
	}
}

// reflectionDefaultForEnv answers whether this environment serves the
// reflection API when GRPC_REFLECTION says nothing.
//
// Reflection publishes the service definitions, so the default has to be "no"
// anywhere they are not already public — which is every environment except the
// ones a developer runs against. GRPC_REFLECTION overrules this in both
// directions: turning it on for a staging deployment being debugged, and off
// for a development deployment that is reachable from outside.
func reflectionDefaultForEnv(envVal *string) bool {
	if envVal == nil {
		return false
	}

	switch env.Environment(*envVal) {
	case env.Local, env.Development:
		return true
	default:
		return false
	}
}

// envName names the environment for a log record, without a nil check at the
// call site.
func envName(envVal *string) string {
	if envVal == nil {
		return ""
	}
	return *envVal
}

func NewGRPCServer(logger loggear.Logger, config *appconfig.GRPC) *ggrpc.Server {
	logCfg := LogConfig(config)
	recCfg := recoverysrv.DefaultRecoveryConfig()
	logUnarySuccess, logUnaryError := CreateBasicUnaryLogFuncs(logger)
	logStreamSuccess, logStreamError := CreateBasicStreamLogFuncs(logger)
	logUnaryPanic, logStreamPanic := CreateBasicPanicLogFuncs(logger)

	// A sample rate of 0 means "log no successful call", but the interceptor
	// cannot be told that: an unset ServerConfig has a zero there too, so it
	// reads 0 as "not configured" and logs every one of them. Honoring the
	// setting is therefore done here, by handing it success loggers that write
	// nothing. Errors still go through logUnaryError / logStreamError.
	if config.LogSuccessSampleRate == 0 {
		logUnarySuccess = func(*logsrv.UnaryInfo) {}
		logStreamSuccess = func(*logsrv.StreamInfo) {}
	}

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
	opts = append(opts, serverLimits(config)...)

	return ggrpc.NewServer(opts...)
}

// serverLimits turns the configured message sizes and keepalive settings into
// server options.
//
// A setting nobody gave is left out rather than passed as zero, so that
// grpc-go's own defaults stay in place. Passing them all would mean this
// template deciding, for every application built from it, what a message size
// limit and a ping interval should be.
func serverLimits(config *appconfig.GRPC) []ggrpc.ServerOption {
	var opts []ggrpc.ServerOption

	if config.MaxRecvMsgSize > 0 {
		opts = append(opts, ggrpc.MaxRecvMsgSize(config.MaxRecvMsgSize))
	}
	if config.MaxSendMsgSize > 0 {
		opts = append(opts, ggrpc.MaxSendMsgSize(config.MaxSendMsgSize))
	}

	// The two halves are passed separately because they are separate options,
	// and each is passed only when something in it was configured. grpc-go
	// fills the remaining zero fields with its own defaults, so a half-filled
	// struct is safe.
	if config.KeepaliveTime > 0 || config.KeepaliveTimeout > 0 {
		opts = append(opts, ggrpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    config.KeepaliveTime,
			Timeout: config.KeepaliveTimeout,
		}))
	}
	if config.KeepaliveMinTime > 0 || config.KeepalivePermitWithoutStream {
		opts = append(opts, ggrpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             config.KeepaliveMinTime,
			PermitWithoutStream: config.KeepalivePermitWithoutStream,
		}))
	}

	return opts
}

// gRPC server lifecycle registration
func RegisterGRPCServerLifecycle(lc dikit.LC, shutdowner dikit.Shutdowner, grpcSrv *ggrpc.Server, config *appconfig.GRPC) {
	if grpcSrv == nil {
		return
	}

	// The address is built once and used by both the listener and the log. Two
	// literals would let the log go on naming a port the server no longer
	// listens on.
	addr := fmt.Sprintf(":%d", config.Port)

	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			listen, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("gRPC server failed to listen: %w", err)
			}
			go func() {
				loggear.Info("gRPC server starting", "addr", addr)
				if err := grpcSrv.Serve(listen); err != nil {
					loggear.Error("gRPC server stopped unexpectedly", "error", err)
					// See the HTTP server: a shutdown without an exit code would
					// end the process with 0 and read as a clean stop.
					_ = shutdowner.Shutdown(dikit.ExitCode(1))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			loggear.Info("Shutting down gRPC server")

			// A gRPC-specific grace period, where one is configured, is applied
			// inside the lifecycle's own deadline rather than beside it: a
			// long-running stream would otherwise be free to spend the whole
			// shutdown budget here and leave none for what is stopped after it.
			// Unset, the deadline is the lifecycle's, exactly as before.
			if config.GracefulStopTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, config.GracefulStopTimeout)
				defer cancel()
			}

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
