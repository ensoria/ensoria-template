package http

import (
	"fmt"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// NewGet は1件のユーザーを id で取得するエンドポイント(型付き Endpoint)。
// GET はボディが無いので Req 型は restkit.NoBody。
//
// パス値 `id` の検証は PathRules に宣言するだけでよい。制約に違反した場合、
// アダプタが自動で 422 + フィールド単位エラー(docai エンベロープ)を返す
// —— 旧実装のように Handle 内で手動でステータス/メッセージを組み立てる必要はない。
func NewGet(svc service.UserService, helloWorld *mbkit.Publication[dto.HelloWorld]) *restkit.Endpoint[restkit.NoBody, dto.GetUser] {
	return &restkit.Endpoint[restkit.NoBody, dto.GetUser]{
		Summary:  "Fetch one user by id",
		Task:     "read user",
		IDPrefix: "usr",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{"users:read"}},
		Produces: rest.MediaTypeXML, // このエンドポイントは XML を返す
		// パスパラメータ id の検証。違反時はアダプタが 422 + field_errors を返す。
		PathRules: []*rule.RuleSet{
			{Field: "id", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.GetUser], error) {
			id, _ := r.PathValue("id")
			_ = id // ここで id を使ってユーザーを取得する

			svc.Something() // DEBUG:
			// 発行は型付きの宣言(mbkit.Publication)を通す。生のmb.Publishを直接呼ぶと、
			// どのドキュメントにも載らないメッセージが飛ぶことになる。
			// リクエストのコンテキストをそのままPublishへ伝播する。
			helloWorld.Publish(r.Context(), &dto.HelloWorld{
				Message: "Hello, World!",
				Source:  "Get.Handle",
			}, nil)

			// 別モジュールの gRPC サービス呼び出し
			if _, err := svc.GetPostContent("1"); err != nil {
				fmt.Println("gRPC call failed:", err)
			}

			return rest.NewResult(&dto.GetUser{ID: 1, Name: "hoge"},
				rest.WithHeader("Server", "net/http")), nil
		},
	}
}
