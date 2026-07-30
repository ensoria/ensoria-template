package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

// deadLetterListLimit is how many dead letter jobs one listing returns, until
// the endpoint supports paging.
const deadLetterListLimit = 100

// NewListDeadLetterJobs lists the jobs that ended up in the dead letter queue
// (typed Endpoint).
//
// TODO: support filtering, sorting and paging through query parameters.
func NewListDeadLetterJobs(wrk *worker.Worker) *restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobList] {
	return &restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobList]{
		Summary:  "List dead letter jobs",
		Task:     "inspect dead letter jobs",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{ScopeJobsRead}},
		FieldDocs: map[string]string{
			"count": "Number of jobs in this response, not the total in the queue",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Related: []string{
			"Retry one of them: POST /_/dead-letter-jobs/{id}/retry",
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.DeadLetterJobList], error) {
			jobs, err := wrk.GetDeadLetterJobs(r.Context(), deadLetterListLimit)
			if err != nil {
				// An unrecognised error becomes 500 without leaking its text.
				return nil, err
			}
			return rest.NewResult(&dto.DeadLetterJobList{Jobs: jobs, Count: len(jobs)}), nil
		},
	}
}

// NewGetDeadLetterJobs fetches one dead letter job by id (typed Endpoint).
//
// TODO: the worker has no single-job lookup yet; implement once it does.
func NewGetDeadLetterJobs(_ *worker.Worker) *restkit.Endpoint[restkit.NoBody, restkit.NoBody] {
	return &restkit.Endpoint[restkit.NoBody, restkit.NoBody]{
		Summary:   "Fetch one dead letter job",
		Task:      "inspect a dead letter job",
		Success:   http.StatusOK,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeJobsRead}},
		PathRules: idRules(),
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotImplemented,
				Code:         CodeNotImplemented,
				Condition:    "Always, until the worker provides a single-job lookup",
				CallerAction: "Use GET /_/dead-letter-jobs instead.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[restkit.NoBody], error) {
			return nil, notImplementedError("fetching one dead letter job is not implemented")
		},
	}
}
