package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

type DeleteDeadLetterJob struct {
	worker *worker.Worker
}

func NewDeleteDeadLetterJob(worker *worker.Worker) *DeleteDeadLetterJob {
	return &DeleteDeadLetterJob{
		worker: worker,
	}
}

func (c *DeleteDeadLetterJob) Handle(r *rest.Request) *rest.Response {
	id, exists := r.PathValue("id")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.JobControlError{Message: "job id is required"},
		}
	}

	ctx := r.Context()
	if err := c.worker.DeleteDeadLetterJob(ctx, id); err != nil {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.JobControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.DeadLetterJobDeleted{Id: id},
	}
}
