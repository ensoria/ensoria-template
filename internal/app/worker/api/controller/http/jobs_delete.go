package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

// NewCancelJob cancels a queued or running job (typed Endpoint).
//
// The response is 204 No Content, so both the request and the response type are
// restkit.NoBody. The previous implementation declared 204 and still wrote a
// body, which a conforming client discards.
func NewCancelJob(wrk *worker.Worker) *restkit.Endpoint[restkit.NoBody, restkit.NoBody] {
	return &restkit.Endpoint[restkit.NoBody, restkit.NoBody]{
		Summary:   "Cancel a job",
		Task:      "cancel a job",
		Success:   http.StatusNoContent,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeJobsWrite}},
		PathRules: idRules(),
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"stops the job from running"},
			// Cancelling an already cancelled job leaves it cancelled.
			Idempotent: new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         CodeJobNotFound,
				Condition:    "No job is stored under that id",
				CallerAction: "Check the id. Do not retry.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[restkit.NoBody], error) {
			id, _ := r.PathValue("id")
			if err := wrk.Cancel(r.Context(), id); err != nil {
				return nil, restkit.NewError(http.StatusNotFound, CodeJobNotFound, err.Error())
			}
			return restkit.NoContent(), nil
		},
	}
}
