package http

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/mw"
	"github.com/ensoria/rest/pkg/pipeline"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsrouter"
	"go.uber.org/fx"
)

// HTTPサーバーの初期化
func NewHTTPApp(envVal *string) func(lc dikit.LC, shutdowner dikit.Shutdowner, httpPipeline *pipeline.HTTP, wsRouter *wsrouter.Router) *http.Server {
	return func(lc dikit.LC, shutdowner dikit.Shutdowner, httpPipeline *pipeline.HTTP, wsRouter *wsrouter.Router) *http.Server {
		httpPipeline.Register()
		wsRouter.Register()

		// TODO: envValを使うこと
		params, err := registry.ModuleParams("default")
		if err != nil {
			log.Fatalf("default config parameters not found: %s", err)
		}
		// FIXME: 別の場所に移す
		loggear.SetLogLevel(params.Log.Level)

		httpSrv := &http.Server{
			Addr: fmt.Sprintf(":%d", params.Server.Port),
		}

		RegisterHTTPServerLifecycle(lc, shutdowner, httpSrv)
		return httpSrv
	}
}

func CreateHTTPPipeline(modules []*rest.Module) *pipeline.HTTP {
	// TODO: 別のファイルに分ける
	panicResponse := &rest.Response{
		Code: http.StatusInternalServerError,
		Body: &dto.Error{Message: "internal server error"},
	}

	configParams, err := registry.ModuleParams("default")
	if err != nil {
		log.Fatalf("default config parameters not found: %s", err)
	}

	cors := &mw.CORSSettings{
		AllowOrigin:      configParams.CORS.AllowOrigin(),
		AllowMethods:     configParams.CORS.AllowMethods(),
		AllowHeaders:     configParams.CORS.AllowHeaders(),
		ExposeHeaders:    configParams.CORS.ExposeHeaders(),
		MaxAge:           configParams.CORS.MaxAge(),
		AllowCredentials: configParams.CORS.AllowCredentials(),
	}

	return &pipeline.HTTP{
		Modules: modules,
		GlobalMiddlewares: []rest.Middleware{
			mw.Logging(logIncomingRequest),
			mw.RecoveryWithLogger(panicResponse, logPanicDetails),
			mw.VerifyBodyParsable,
			mw.NewCORS(cors),
		},
	}
}

// HTTP/WebSocket controllers lifecycle registration
func RegisterHTTPServerLifecycle(lc dikit.LC, shutdowner dikit.Shutdowner, srv *http.Server) {
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
			loggear.Info("Shutting down HTTP server")
			return srv.Shutdown(ctx)
		},
	})
}

func InjectHTTPModules(f any) any {
	return fx.Annotate(f, fx.ParamTags(dikit.GroupTagHttpModules))
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
