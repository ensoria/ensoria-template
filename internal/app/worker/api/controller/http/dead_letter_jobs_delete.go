package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

// NewDeleteDeadLetterJob removes one job from the dead letter queue
// (typed Endpoint). The job is discarded, not retried.
func NewDeleteDeadLetterJob(wrk *worker.Worker) *restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobDeleted] {
	return &restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobDeleted]{
		Summary:   "Delete one dead letter job",
		Task:      "discard a dead letter job",
		Success:   http.StatusOK,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeJobsWrite}},
		PathRules: idRules(),
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"discards the job; it will never run again"},
			Idempotent:  new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         CodeJobNotFound,
				Condition:    "No dead letter job is stored under that id",
				CallerAction: "Check the id against GET /_/dead-letter-jobs. Do not retry.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.DeadLetterJobDeleted], error) {
			id, _ := r.PathValue("id")
			if err := wrk.DeleteDeadLetterJob(r.Context(), id); err != nil {
				return nil, restkit.NewError(http.StatusNotFound, CodeJobNotFound, err.Error())
			}
			return rest.NewResult(&dto.DeadLetterJobDeleted{Id: id}), nil
		},
	}
}
