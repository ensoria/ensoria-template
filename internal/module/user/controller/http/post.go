package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// NewPost はユーザー作成エンドポイントを返す(型付き Endpoint)。
// リクエスト/レスポンスの型・検証ルール・ステータス・振る舞いを宣言面に持つため、
// apidoc がこれらをリフレクションして docai を生成できる。
func NewPost(svc service.UserService) *restkit.Endpoint[dto.CreateUser, dto.CreateUser] {
	return &restkit.Endpoint[dto.CreateUser, dto.CreateUser]{
		Summary:  "Create a user",
		IDPrefix: "usr",
		Success:  http.StatusCreated,
		BodyRules: []*rule.RuleSet{
			{Field: "name", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}},
		},
		FieldDocs: map[string]string{
			"name": "User display name",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
		},
		Handle: func(r *rest.Request, req *dto.CreateUser) (*rest.Result[dto.CreateUser], error) {
			// ここで svc.Create(req) を呼ぶ
			_ = svc
			return rest.NewResult(&dto.CreateUser{ID: 1, Name: req.Name}), nil
		},
	}
}
