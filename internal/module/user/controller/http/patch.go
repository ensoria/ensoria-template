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

// NewPatch はユーザーを部分更新するエンドポイント(型付き Endpoint)。
//
// 部分更新は「そのフィールドに触れない」と「そのフィールドを空にする」を区別する
// 必要がある。JSON では `{}` と `{"nickname": null}` の差であり、ポインタでは
// どちらも nil になって表せない。そのため dto.UpdateUser の各フィールドは
// optional.Optional になっている。
//
// 宣言の意味はフィールドごとに違う:
//   - name     … 触れなくてよいが、送るなら値が要る(NotNullIfSet が null を拒否)
//   - nickname … 触れなくてもよいし、null を送って消してもよい
func NewPatch(svc service.UserService) *restkit.Endpoint[dto.UpdateUser, dto.GetUser] {
	return &restkit.Endpoint[dto.UpdateUser, dto.GetUser]{
		Summary:  "Update parts of a user",
		Task:     "update user",
		IDPrefix: "usr",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{"users:write"}},
		PathRules: []*rule.RuleSet{
			{Field: "id", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}},
		},
		BodyRules: []*rule.RuleSet{
			// 送られてきた場合のみ検証される。触れなければ何も言わない。
			{Field: "name", Rules: []rule.Rule{vkit.NotNullIfSet(), vkit.MaxLength(10)}},
		},
		// フィールドそのものの意味は両方に当てる。
		FieldDocs: map[string]string{
			"name": "User display name",
		},
		// 送り方の話はリクエストにだけ当てる。同じ `name` がレスポンスにもあるので、
		// FieldDocs に書くと「省略すると変わらない」がレスポンス表にも出てしまう。
		RequestFieldDocs: map[string]string{
			"name":     "User display name. Omit to leave it unchanged; it cannot be cleared",
			"nickname": "New nickname. Omit to leave it unchanged, or send null to clear it",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"changes only the fields present in the request"},
			// 同じ本文を二度送っても結果は同じ。
			Idempotent: new(true),
		},
		Related: []string{
			"Read the result: GET /users/{id}",
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         "user_not_found",
				Condition:    "No user exists under that id",
				CallerAction: "Check the id. Do not retry.",
			},
		},
		Handle: func(r *rest.Request, body *dto.UpdateUser) (*rest.Result[dto.GetUser], error) {
			id, _ := r.PathValue("id")
			_ = id
			_ = svc

			// 触れられたフィールドだけを反映する。IsSet を見ないと、送られなかった
			// フィールドをゼロ値で上書きしてしまう。
			user := &dto.GetUser{ID: 1, Name: "hoge"}
			if name, ok := body.Name.Get(); ok {
				user.Name = name
			}
			// nickname は null で消せる。IsSet が true で値が無ければ「消す」指示。
			if body.Nickname.IsSet() {
				// ここで svc に「消す」または「値を入れる」を伝える
				_ = body.Nickname.OrElse("")
			}

			return rest.NewResult(user), nil
		},
	}
}
