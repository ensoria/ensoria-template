package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/worker/pkg/worker"
)

// idRules validates the `id` path value. Declaring it here means the adapter
// answers 422 with field errors before Handle runs, so no endpoint has to check
// for a missing job id itself.
func idRules() []*rule.RuleSet {
	return []*rule.RuleSet{
		{Field: "id", Rules: []rule.Rule{vkit.Required()}},
	}
}

// notImplementedError reports a feature the worker does not provide yet.
func notImplementedError(what string) error {
	return restkit.NewError(http.StatusNotImplemented, CodeNotImplemented, what)
}

// NewListJobs lists jobs (typed Endpoint).
//
// TODO: the worker has no listing feature yet; implement once it does.
// TODO: support filtering, sorting and paging through query parameters.
func NewListJobs(_ *worker.Worker) *restkit.Endpoint[restkit.NoBody, restkit.NoBody] {
	return &restkit.Endpoint[restkit.NoBody, restkit.NoBody]{
		Summary:  "List jobs",
		Task:     "inspect jobs",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{ScopeJobsRead}},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotImplemented,
				Code:         CodeNotImplemented,
				Condition:    "Always, until the worker provides a listing feature",
				CallerAction: "Do not call this endpoint yet.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[restkit.NoBody], error) {
			return nil, notImplementedError("listing jobs is not implemented")
		},
	}
}

// NewJobStatus fetches the status of one job by id (typed Endpoint).
func NewJobStatus(wrk *worker.Worker) *restkit.Endpoint[restkit.NoBody, dto.JobStatus] {
	return &restkit.Endpoint[restkit.NoBody, dto.JobStatus]{
		Summary:   "Fetch the status of one job",
		Task:      "inspect a job",
		Success:   http.StatusOK,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeJobsRead}},
		PathRules: idRules(),
		FieldDocs: map[string]string{
			"status": "One of the worker job statuses (queued, running, succeeded, failed)",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         CodeJobNotFound,
				Condition:    "No job is stored under that id",
				CallerAction: "Check the id. Do not retry.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.JobStatus], error) {
			id, _ := r.PathValue("id")
			status, err := wrk.GetJobStatus(r.Context(), id)
			if err != nil {
				return nil, restkit.NewError(http.StatusNotFound, CodeJobNotFound, err.Error())
			}
			return rest.NewResult(&dto.JobStatus{Id: id, Status: string(status)}), nil
		},
	}
}
