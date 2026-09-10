package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
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

func CreateHTTPPipeline(envVal *string) func(
	modules []*rest.Module,
	verifier authkit.Verifier,
	origins *middleware.Origins,
	scopes authkit.ScopeExpander,
) (*pipeline.HTTP, error) {
	return func(
		modules []*rest.Module,
		verifier authkit.Verifier,
		origins *middleware.Origins,
		scopes authkit.ScopeExpander,
	) (*pipeline.HTTP, error) {
		return createHTTPPipeline(*envVal, modules, verifier, origins, scopes)
	}
}

func createHTTPPipeline(
	envVal string,
	modules []*rest.Module,
	verifier authkit.Verifier,
	origins *middleware.Origins,
	scopes authkit.ScopeExpander,
) (*pipeline.HTTP, error) {
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
	//
	// The session checks run first. They are the specific case of the same
	// mistake, and their messages name the wiring rather than the symptom.
	if err := checkSessionConfiguration(envVal, configParams, modules); err != nil {
		return nil, err
	}
	if err := checkAuthConfiguration(modules, verifier); err != nil {
		return nil, err
	}

	// CORS, this check and the WebSocket upgrade guard are all handed the same
	// resolved origins (see ws.NewTrustedOrigins) rather than each reading
	// CORS_ALLOW_ORIGIN for themselves.
	crossOrigin, err := middleware.NewCrossOriginProtection(origins)
	if err != nil {
		return nil, err
	}

	// Layer 2: リクエスト単位（ハンドラ実行）のタイムアウト超過時に返すレスポンス
	timeoutResponse := &rest.Response{
		Code: http.StatusServiceUnavailable,
		Body: &dto.Error{Message: "request timeout"},
	}

	return &pipeline.HTTP{
		Modules: modules,
		GlobalMiddlewares: globalMiddlewares(&globalMiddlewareDeps{
			cors:          configParams.CORS,
			crossOrigin:   crossOrigin,
			verifier:      verifier,
			scopes:        scopes,
			panicResponse: panicResponse,
		}),
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
					// The exit code has to say the process failed. Shutting down
					// without one ends the process with 0, and a supervisor that
					// restarts on failure (systemd Restart=on-failure, a
					// Kubernetes restartPolicy) reads that as a clean stop and
					// leaves the application down.
					_ = shutdowner.Shutdown(dikit.ExitCode(1))
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
func InjectHTTPModules(f any) any {
	return fx.Annotate(f, fx.ParamTags(dikit.GroupTagHttpModules, ``))
}
