package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

type CancelJob struct {
	worker *worker.Worker
}

func NewCancelJob(worker *worker.Worker) *CancelJob {
	return &CancelJob{
		worker: worker,
	}
}

func (c *CancelJob) Handle(r *rest.Request) *rest.Response {
	jobId, exists := r.PathValue("id")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.JobControlError{Message: "job id is required"},
		}
	}

	ctx := r.Context()
	if err := c.worker.Cancel(ctx, jobId); err != nil {
		return &rest.Response{
			Code: http.StatusNotFound,
			Body: &dto.JobControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusNoContent,
		Body: dto.JobCancelled{Id: jobId},
	}
}
