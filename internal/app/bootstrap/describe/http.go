package describe

import (
	"fmt"
	"reflect"

	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	"github.com/ensoria/ensoria-template/internal/app/apiinfo"
	httpdto "github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/rest/pkg/rest"
	"go.uber.org/fx"
)

// BuildHTTP は HTTP モジュールを実インフラなしで解決し、APISpec を返す。
func BuildHTTP(envVal *string) (*apidoc.APISpec, error) {
	if err := registry.InitializeConfiguration(*envVal, assets.ConfigFS(*envVal), "internal", "config"); err != nil {
		return nil, fmt.Errorf("app initialization error: %w", err)
	}

	modules, err := resolveHTTPModules()
	if err != nil {
		return nil, err
	}

	spec := apidoc.Build(modules)
	spec.Info = apiinfo.Info()
	spec.Conventions = buildConventions()
	return spec, nil
}

// resolveHTTPModules は fx で `http_modules` group だけを解決する。
// アプリが提供する infra 型は stubs.go の一覧をそのまま Provide し、実 infra は
// 登録しない(repository は db 非依存、gRPC は grpc.NewClient で遅延接続のため
// 接続は走らない)。
// `.Run()` / `.Start()` は呼ばないので OnStart ライフサイクルも実行されない。
func resolveHTTPModules() ([]*rest.Module, error) {
	var modules []*rest.Module

	app := fx.New(
		fx.Provide(dikit.Constructors()...),
		fx.Provide(stubs()...),
		fx.Populate(fx.Annotate(&modules, fx.ParamTags(dikit.GroupTagHttpModules))),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		return nil, withStubHint(fmt.Errorf("describe: resolve http modules: %w", err))
	}
	return modules, nil
}

// buildConventions は config / pipeline 由来の共通規約を集める。
func buildConventions() *apidoc.Conventions {
	conv := &apidoc.Conventions{
		CommonError:       apidoc.CommonErrorSchema(reflect.TypeOf(httpdto.Error{})),
		GlobalMiddlewares: []string{"logging", "recovery", "verify-body-parsable", "cors"},
	}

	params, err := registry.ModuleParams("default")
	if err != nil {
		return conv
	}
	conv.BaseURLs = map[string]string{
		"local": fmt.Sprintf("http://localhost:%d", params.Server.Port),
	}
	conv.SecuritySchemes = securitySchemes(params.Auth)
	conv.CORS = &apidoc.CORS{
		AllowOrigin:      params.CORS.AllowOrigin(),
		AllowMethods:     params.CORS.AllowMethods(),
		AllowHeaders:     params.CORS.AllowHeaders(),
		ExposeHeaders:    params.CORS.ExposeHeaders(),
		AllowCredentials: params.CORS.AllowCredentials(),
		MaxAge:           params.CORS.MaxAge(),
	}
	return conv
}
