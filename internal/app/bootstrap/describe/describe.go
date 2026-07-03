// Package describe は、実インフラに接続せずに HTTP モジュールを DI で解決し、
// apidoc.APISpec を組み立てる「describe モード」を提供する。
//
// docai / OpenAPI 生成のために、サーバを起動せずルーティング・型・宣言メタだけを
// 取り出す。DB/MB などの接続系はスタブを注入し、fx のライフサイクル(OnStart)は
// 起動しない(= ポート bind も接続も走らない)。
package describe

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	httpdto "github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/job"
	"github.com/ensoria/worker/pkg/worker"
	"go.uber.org/fx"

	// モジュールの init() でコンストラクタ(repository/service/controller/module)を登録する。
	_ "github.com/ensoria/ensoria-template/internal/module"
)

// Build は HTTP モジュールを実インフラなしで解決し、APISpec を返す。
func Build(envVal *string) (*apidoc.APISpec, error) {
	registry.InitializeConfiguration(envVal, assets.ConfigFS(*envVal), "internal", "config")

	modules, err := resolveHTTPModules()
	if err != nil {
		return nil, err
	}

	spec := apidoc.Build(modules)
	spec.Conventions = buildConventions()
	return spec, nil
}

// resolveHTTPModules は fx で `http_modules` group だけを解決する。
// 接続系(mb.Publish / worker.Enqueuer)はスタブを Provide し、実 infra は登録しない
// (repository は db 非依存、gRPC は grpc.NewClient で遅延接続のため接続は走らない)。
// `.Run()` / `.Start()` は呼ばないので OnStart ライフサイクルも実行されない。
func resolveHTTPModules() ([]*rest.Module, error) {
	var modules []*rest.Module

	app := fx.New(
		fx.Provide(dikit.Constructors()...),
		fx.Provide(
			func() mb.Publish { return stubPublish },
			func() worker.Enqueuer { return stubEnqueuer{} },
		),
		fx.Populate(fx.Annotate(&modules, fx.ParamTags(dikit.GroupTagHttpModules))),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		return nil, fmt.Errorf("describe: resolve http modules: %w", err)
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

// --- describe 用のスタブ(接続を張らない) ---

var stubPublish mb.Publish = func(target string, data []byte, metadata map[string]string, opts ...mb.PublishOption) error {
	return nil
}

type stubEnqueuer struct{}

func (stubEnqueuer) Enqueue(ctx context.Context, jobName string, payload any, opts ...*job.Option) (string, error) {
	return "", nil
}
