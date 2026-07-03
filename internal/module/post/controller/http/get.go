package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/post/dto"
	"github.com/ensoria/ensoria-template/internal/module/post/service"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// NewGet は1件の投稿を取得するエンドポイント(型付き Endpoint)。
// GET はボディが無いので Req 型は restkit.NoBody。レスポンスは dto.Post(構造体)なので
// docai のレスポンス表・example は型から自動生成される。
func NewGet(svc service.PostService) *restkit.Endpoint[restkit.NoBody, dto.Post] {
	return &restkit.Endpoint[restkit.NoBody, dto.Post]{
		Summary:  "Fetch a post",
		IDPrefix: "pst",
		Success:  http.StatusOK,
		Produces: rest.MediaTypeXML, // このエンドポイントは XML を返す
		FieldDocs: map[string]string{
			"Title":   "Post title",
			"Content": "Post body",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  boolPtr(true),
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.Post], error) {
			return rest.NewResult(svc.GetPost(),
				rest.WithHeader("Server", "net/http")), nil
		},
	}
}

func boolPtr(b bool) *bool { return &b }
