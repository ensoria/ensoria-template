package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/mw"
	"github.com/ensoria/rest/pkg/pipeline"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconn"
	"github.com/ensoria/websocket/pkg/wsrouter"
	"go.uber.org/fx"
)

// HTTPサーバーの初期化
//
// A failure is returned rather than written with log.Fatal: fx reports it
// through the one path every other startup failure takes, and the lifecycle
// still gets to stop what had already started — os.Exit would leave the
// connections opened before this point unclosed.
func NewHTTPApp(envVal *string) func(lc dikit.LC, shutdowner dikit.Shutdowner, httpPipeline *pipeline.HTTP, wsRouter *wsrouter.Router) (*http.Server, error) {
	return func(lc dikit.LC, shutdowner dikit.Shutdowner, httpPipeline *pipeline.HTTP, wsRouter *wsrouter.Router) (*http.Server, error) {
		// TODO: envValを使うこと
		params, err := registry.ModuleParams("default")
		if err != nil {
			return nil, fmt.Errorf("default config parameters not found: %w", err)
		}

		// HTTPパイプラインとWebSocketルータを同一のmuxに登録する。
		// グローバルなhttp.DefaultServeMuxを使わないことで、ハンドラを分離でき、
		// テストの並列化や複数サーバの起動が可能になる。
		mux := http.NewServeMux()
		httpPipeline.Register(mux)
		wsRouter.Register(mux)

		httpSrv := &http.Server{
			Addr:    fmt.Sprintf(":%d", params.Server.Port),
			Handler: mux,
			// Layer 1: コネクションレベルのタイムアウト（configから取得）
			ReadHeaderTimeout: params.Server.ReadHeaderTimeout,
			ReadTimeout:       params.Server.ReadTimeout,
			// WriteTimeoutはレスポンス書き込み全体の絶対deadlineであり、SSE・WebSocket・
			// 大きなファイルのような長時間接続を切断する。そのため既定では0(無効)。
			// リクエスト単位のタイムアウトはpipeline側(Layer 2)で制御する。
			WriteTimeout: params.Server.WriteTimeout,
			IdleTimeout:  params.Server.IdleTimeout,
		}

		RegisterHTTPServerLifecycle(lc, shutdowner, httpSrv, wsRouter)
		return httpSrv, nil
	}
}

func CreateHTTPPipeline(modules []*rest.Module, verifier authkit.Verifier) (*pipeline.HTTP, error) {
	// TODO: 別のファイルに分ける
	panicResponse := &rest.Response{
		Code: http.StatusInternalServerError,
		Body: &dto.Error{Message: "internal server error"},
	}

	configParams, err := registry.ModuleParams("default")
	if err != nil {
		return nil, fmt.Errorf("default config parameters not found: %w", err)
	}

	// 宣言と設定の食い違いを起動時に潰す。放っておくと全リクエストが拒否されるだけで、
	// 原因が見えない。
	if err := checkAuthConfiguration(modules, verifier); err != nil {
		return nil, err
	}

	cors := &mw.CORSSettings{
		AllowOrigin:      configParams.CORS.AllowOrigin(),
		AllowMethods:     configParams.CORS.AllowMethods(),
		AllowHeaders:     configParams.CORS.AllowHeaders(),
		ExposeHeaders:    configParams.CORS.ExposeHeaders(),
		MaxAge:           configParams.CORS.MaxAge(),
		AllowCredentials: configParams.CORS.AllowCredentials(),
	}

	// Layer 2: リクエスト単位（ハンドラ実行）のタイムアウト超過時に返すレスポンス
	timeoutResponse := &rest.Response{
		Code: http.StatusServiceUnavailable,
		Body: &dto.Error{Message: "request timeout"},
	}

	return &pipeline.HTTP{
		Modules:           modules,
		GlobalMiddlewares: globalMiddlewares(cors, verifier, panicResponse),
		// Layer 2: コントローラ/ミドルウェアチェーンの実行（=レスポンスの計算）の上限時間。
		// 0で無効。ストリーミング/ファイル/WebSocketは対象外。
		Timeout:         configParams.Server.HandlerTimeout,
		TimeoutResponse: timeoutResponse,
	}, nil
}

// HTTP/WebSocket controllers lifecycle registration
func RegisterHTTPServerLifecycle(lc dikit.LC, shutdowner dikit.Shutdowner, srv *http.Server, wsRouter *wsrouter.Router) {
	lc.Append(dikit.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				loggear.Info("HTTP server starting", "addr", srv.Addr)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					loggear.Error("HTTP server stopped unexpectedly", "error", err)
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// WebSocketを先に閉じる。Upgradeでハイジャックされた接続は
			// http.Serverの管理外なので、srv.Shutdownは待っても閉じてもくれない。
			// wsRouter.Shutdownが各サーバのbaseCtxをキャンセルして全接続の
			// connCtxに伝播させ（進行中の処理を中断可能にし）、close frameを
			// 送って読み取りループを解く。各接続のOnCloseは接続ctxとは切り離された
			// ctx（OnCloseTimeout）で後始末を完走できる。
			closed := wsRouter.Shutdown(wsconn.CloseGoingAway, "server shutting down")
			loggear.Info("Closed WebSocket connections", "count", closed)

			loggear.Info("Shutting down HTTP server")
			return srv.Shutdown(ctx)
		},
	})
}

// InjectHTTPModules tags the first parameter as the HTTP module group. The
// remaining parameters (the credential verifier) are resolved by type.
// globalMiddlewares builds the chain every request passes through.
//
// The list runs outside-in, so authentication sits last: logging, panic recovery
// and CORS still apply to a request that is refused, and a CORS preflight (which
// carries no credential) is answered before authentication is considered.
func globalMiddlewares(cors *mw.CORSSettings, verifier authkit.Verifier, panicResponse *rest.Response) []rest.Middleware {
	return []rest.Middleware{
		mw.Logging(logIncomingRequest),
		mw.RecoveryWithLogger(panicResponse, logPanicDetails),
		mw.VerifyBodyParsable,
		mw.NewCORS(cors),
		middleware.Auth(verifier),
	}
}

func InjectHTTPModules(f any) any {
	return fx.Annotate(f, fx.ParamTags(dikit.GroupTagHttpModules, ``))
}

func logIncomingRequest(req *rest.Request, res *rest.Response) {
	loggear.Info("HTTP Request",
		slog.String("method", req.Method()),
		slog.String("path", req.Path()),
		slog.Int("status_code", res.Code),
		slog.String("remote_addr", req.RemoteAddr()),
		slog.String("user_agent", req.UserAgent()),
		slog.String("type", "access_log"),
	)
}

func logPanicDetails(r *rest.Request, panicValue interface{}, stackTrace []byte) {
	loggear.Error("Panic Recovered",
		slog.String("method", r.Method()),
		slog.String("url", r.URLStr()),
		slog.String("remote_addr", r.RemoteAddr()),
		slog.String("user_agent", r.UserAgent()),
		slog.Any("panic_value", panicValue),
		slog.String("panic_type", fmt.Sprintf("%T", panicValue)),
		slog.String("stack_trace", string(stackTrace)),
		slog.String("type", "panic_log"),
	)
}
