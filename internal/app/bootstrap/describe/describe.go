// Package describe は、実インフラに接続せずに HTTP モジュールを DI で解決し、
// apidoc.APISpec を組み立てる「describe モード」を提供する。
//
// docai / OpenAPI 生成のために、サーバを起動せずルーティング・型・宣言メタだけを
// 取り出す。DB/MB などの接続系はスタブを注入し、fx のライフサイクル(OnStart)は
// 起動しない(= ポート bind も接続も走らない)。
package describe

import (
	"fmt"
	"reflect"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	"github.com/ensoria/ensoria-template/internal/app/apiinfo"
	httpdto "github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/rest/pkg/rest"
	"go.uber.org/fx"

	// モジュールの init() でコンストラクタ(repository/service/controller/module)を登録する。
	// アプリが配信するものと同じ集合を取り込む —— 取りこぼすと、実際には存在する
	// エンドポイントが生成ドキュメントから静かに消える。
	_ "github.com/ensoria/ensoria-template/internal/app/scheduler/api"
	_ "github.com/ensoria/ensoria-template/internal/app/worker/api"
	_ "github.com/ensoria/ensoria-template/internal/module"
	_ "github.com/ensoria/ensoria-template/internal/query"
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

// securitySchemes は設定されている検証手段から、呼び出し元が使える資格情報の方式を組む。
// 設定されていない方式は出さない —— 使えない認証方法をドキュメントに載せないため。
//
// API キーは、設定に並んでいる場合と「別の場所で検証する」と宣言されている場合の
// 両方で出す。DB から検証するアプリでも、呼び出し元にとっては API キーが使えることに
// 変わりないため。
func securitySchemes(auth *appconfig.Auth) []apidoc.SecurityScheme {
	if auth == nil {
		return nil
	}

	var schemes []apidoc.SecurityScheme
	if auth.Secret != "" || auth.JWKSURL != "" {
		schemes = append(schemes, apidoc.SecurityScheme{
			Name:         authkit.SchemeJWT,
			Type:         apidoc.SecuritySchemeTypeHTTP,
			Scheme:       apidoc.SecuritySchemeBearer,
			BearerFormat: apidoc.BearerFormatJWT,
			Description:  "Bearer token issued by the identity provider",
		})
	}
	if auth.AcceptsAPIKeys() {
		header := auth.APIKeyHeader
		if header == "" {
			header = appconfig.DefaultAPIKeyHeader
		}
		schemes = append(schemes, apidoc.SecurityScheme{
			Name:        authkit.SchemeAPIKey,
			Type:        apidoc.SecuritySchemeTypeAPIKey,
			In:          apidoc.SecuritySchemeInHeader,
			HeaderName:  header,
			Description: "Key issued to a machine caller",
		})
	}
	return schemes
}
