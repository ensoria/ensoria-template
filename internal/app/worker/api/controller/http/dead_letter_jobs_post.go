package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/worker/pkg/worker"
)

type RetryDeadLetterJob struct {
	worker *worker.Worker
}

func NewRetryDeadLetterJob(worker *worker.Worker) *RetryDeadLetterJob {
	return &RetryDeadLetterJob{
		worker: worker,
	}
}

func (c *RetryDeadLetterJob) Handle(r *rest.Request) *rest.Response {
	jobID, exists := r.PathValue("id")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.JobControlError{Message: "job id is required"},
		}
	}

	ctx := r.Context()
	if err := c.worker.RetryDeadLetterJob(ctx, jobID); err != nil {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.JobControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.DeadLetterJobRetry{
			Id:      jobID,
			Message: "Job retried successfully",
		},
	}
}

type RetryDeadLetterJobsByName struct {
	worker *worker.Worker
}

func NewRetryDeadLetterJobsByName(worker *worker.Worker) *RetryDeadLetterJobsByName {
	return &RetryDeadLetterJobsByName{
		worker: worker,
	}
}

func (c *RetryDeadLetterJobsByName) Handle(r *rest.Request) *rest.Response {
	reqBody, msgs := vkit.RestRequestBody[dto.RetryByName](
		r,
		&rule.RuleSet{Field: "jobName", Rules: []rule.Rule{vkit.Required()}},
	)
	if msgs != nil {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: msgs,
		}
	}

	ctx := r.Context()
	count, err := c.worker.RetryDeadLetterJobsByName(ctx, reqBody.JobName)
	if err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.JobControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.DeadLetterJobRetry{
			Id:         reqBody.JobName,
			Message:    "jobs retried successfully",
			RetryCount: count,
		},
	}
}

type RetryAllDeadLetterJobs struct {
	worker *worker.Worker
}

func NewRetryAllDeadLetterJobs(worker *worker.Worker) *RetryAllDeadLetterJobs {
	return &RetryAllDeadLetterJobs{
		worker: worker,
	}
}

func (c *RetryAllDeadLetterJobs) Handle(r *rest.Request) *rest.Response {
	ctx := r.Context()
	count, err := c.worker.RetryAllDeadLetterJobs(ctx)
	if err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.JobControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.DeadLetterJobRetry{
			Message:    "all jobs retried successfully",
			RetryCount: count,
		},
	}
}
