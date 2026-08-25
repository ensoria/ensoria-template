package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/ensoria-template/internal/query/user_post/dto"
	"github.com/ensoria/ensoria-template/internal/query/user_post/service"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// NewGet reads the posts of one user (typed Endpoint).
//
// This endpoint declares no Security, which means a verified caller is
// required: an endpoint whose author never decided who may call it ends up
// closed rather than open.
func NewGet(svc service.UserPostService) *restkit.Endpoint[restkit.NoBody, dto.GetUserPost] {
	return &restkit.Endpoint[restkit.NoBody, dto.GetUserPost]{
		Summary:  "Fetch the posts of one user",
		Task:     "read the posts of a user",
		IDPrefix: "usr",
		Success:  http.StatusOK,
		PathRules: []*rule.RuleSet{
			{Field: "id", Rules: []rule.Rule{vkit.Required()}},
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.GetUserPost], error) {
			// The request's context is what the read is cancelled with: it
			// reaches the cache and, on a miss, the repository behind it, so a
			// caller that goes away stops the work it started.
			//
			// TODO: read the id from the path instead of the fixed value below.
			user, err := svc.GetByID(r.Context(), 1)
			if err != nil {
				return nil, err
			}

			return rest.NewResult(&dto.GetUserPost{ID: user.ID}), nil
		},
	}
}
