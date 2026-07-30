package http

import (
	"net/http"
	"reflect"

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
		Task:     "create user",
		IDPrefix: "usr",
		Success:  http.StatusCreated,
		Security: &restkit.SecuritySpec{Scopes: []string{"users:write"}},
		BodyRules: []*rule.RuleSet{
			{Field: "name", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}},
		},
		FieldDocs: map[string]string{
			"name": "User display name",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
		},
		// 前後に呼ぶ関連エンドポイント(§4.1 Related)。
		Related: []string{
			"Fetch after creation: GET /users/{id}",
		},
		// 主レスポンス(201)以外の成功レスポンス。
		// Handle が返し得るステータスはすべてここに宣言する必要がある —— 宣言していない
		// ステータスを返すと、開発環境ではアダプタが即座に失敗させる(宣言漏れの検出)。
		Responses: []restkit.ResponseSpec{
			{
				Status:   http.StatusAccepted,
				When:     "The user was queued for asynchronous creation",
				BodyType: reflect.TypeFor[dto.CreateUser](),
				Headers: []restkit.HeaderSpec{
					{Name: "Retry-After", Meaning: "Seconds to wait before fetching the created user"},
				},
			},
		},
		// このエンドポイント固有のエラー。共通形に従うものは表の1行のみ、
		// field-level を返す 422 は個別 example + 表を出す(BodyType 宣言)。
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusConflict,
				Code:         "email_taken",
				Condition:    "A user with the same email already exists",
				CallerAction: "Use another email. Do not retry.",
			},
			{
				Status:       http.StatusUnprocessableEntity,
				Code:         "validation_failed",
				Condition:    "A request field is invalid",
				CallerAction: "Show the field-level errors in the form. Do not retry.",
				BodyType:     reflect.TypeFor[restkit.ErrorEnvelope](),
			},
		},
		Handle: func(r *rest.Request, req *dto.CreateUser) (*rest.Result[dto.CreateUser], error) {
			// ここで svc.Create(req) を呼ぶ
			_ = svc

			// 非同期で作成する経路では 202 を返す(Responses に宣言済み)。
			// 宣言していないステータスを WithStatus に渡すと、開発環境では即座に失敗する。
			if async, ok := r.Query("async"); ok && async == "true" {
				return rest.NewResult(&dto.CreateUser{ID: 1, Name: req.Name},
					rest.WithStatus(http.StatusAccepted),
					rest.WithHeader("Retry-After", "5")), nil
			}
			return rest.NewResult(&dto.CreateUser{ID: 1, Name: req.Name}), nil
		},
	}
}
