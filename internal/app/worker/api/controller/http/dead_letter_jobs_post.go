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

// retryBehavior describes what a retry does. Retrying is not idempotent: each
// call puts the job back on the queue, so a repeated call runs it again.
func retryBehavior(effect string) restkit.BehaviorSpec {
	return restkit.BehaviorSpec{
		SideEffects: []string{effect},
		Idempotent:  new(false),
	}
}

// NewRetryDeadLetterJob puts one dead letter job back on the queue
// (typed Endpoint).
func NewRetryDeadLetterJob(wrk *worker.Worker) *restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobRetry] {
	return &restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobRetry]{
		Summary:   "Retry one dead letter job",
		Task:      "retry a dead letter job",
		Success:   http.StatusOK,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeJobsWrite}},
		PathRules: idRules(),
		Behavior:  retryBehavior("queues the job to run again"),
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         CodeJobNotFound,
				Condition:    "No dead letter job is stored under that id",
				CallerAction: "Check the id against GET /_/dead-letter-jobs. Do not retry.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.DeadLetterJobRetry], error) {
			id, _ := r.PathValue("id")
			if err := wrk.RetryDeadLetterJob(r.Context(), id); err != nil {
				return nil, restkit.NewError(http.StatusNotFound, CodeJobNotFound, err.Error())
			}
			return rest.NewResult(&dto.DeadLetterJobRetry{
				Id:      id,
				Message: "Job retried successfully",
			}), nil
		},
	}
}

// NewRetryDeadLetterJobsByName retries every dead letter job registered under
// one job name (typed Endpoint).
func NewRetryDeadLetterJobsByName(wrk *worker.Worker) *restkit.Endpoint[dto.RetryByName, dto.DeadLetterJobRetry] {
	return &restkit.Endpoint[dto.RetryByName, dto.DeadLetterJobRetry]{
		Summary:  "Retry every dead letter job with one job name",
		Task:     "retry dead letter jobs by name",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{ScopeJobsWrite}},
		BodyRules: []*rule.RuleSet{
			{Field: "jobName", Rules: []rule.Rule{vkit.Required()}},
		},
		FieldDocs: map[string]string{
			"jobName":      "Name the job was registered under, not a job id",
			"retriedCount": "How many jobs were queued again",
		},
		Behavior: retryBehavior("queues every matching job to run again"),
		Handle: func(r *rest.Request, body *dto.RetryByName) (*rest.Result[dto.DeadLetterJobRetry], error) {
			count, err := wrk.RetryDeadLetterJobsByName(r.Context(), body.JobName)
			if err != nil {
				// An unrecognised error becomes 500 without leaking its text.
				return nil, err
			}
			return rest.NewResult(&dto.DeadLetterJobRetry{
				Id:         body.JobName,
				Message:    "jobs retried successfully",
				RetryCount: count,
			}), nil
		},
	}
}

// NewRetryAllDeadLetterJobs retries every job in the dead letter queue
// (typed Endpoint).
func NewRetryAllDeadLetterJobs(wrk *worker.Worker) *restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobRetry] {
	return &restkit.Endpoint[restkit.NoBody, dto.DeadLetterJobRetry]{
		Summary:   "Retry every dead letter job",
		Task:      "retry all dead letter jobs",
		Success:   http.StatusOK,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeJobsWrite}},
		Behavior:  retryBehavior("queues every dead letter job to run again"),
		FieldDocs: map[string]string{"retriedCount": "How many jobs were queued again"},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.DeadLetterJobRetry], error) {
			count, err := wrk.RetryAllDeadLetterJobs(r.Context())
			if err != nil {
				return nil, err
			}
			return rest.NewResult(&dto.DeadLetterJobRetry{
				Message:    "all jobs retried successfully",
				RetryCount: count,
			}), nil
		},
	}
}
