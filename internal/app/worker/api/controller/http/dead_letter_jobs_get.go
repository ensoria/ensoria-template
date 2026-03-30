package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

type ListDeadLetterJobs struct {
	worker *worker.Worker
}

func NewListDeadLetterJobs(worker *worker.Worker) *ListDeadLetterJobs {
	return &ListDeadLetterJobs{
		worker: worker,
	}
}

// TODO: パラメータなどで絞れたり、ソート、ページングができるようにする
func (c *ListDeadLetterJobs) Handle(r *rest.Request) *rest.Response {
	ctx := r.Context()
	jobs, err := c.worker.GetDeadLetterJobs(ctx, 100)
	if err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.JobControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.DeadLetterJobList{
			Jobs:  jobs,
			Count: len(jobs),
		},
	}

}

type GetDeadLetterJobs struct {
	worker *worker.Worker
}

func NewGetDeadLetterJobs(worker *worker.Worker) *GetDeadLetterJobs {
	return &GetDeadLetterJobs{
		worker: worker,
	}
}

// TODO: workerで一覧機能が未実装のため、workerでの実行完了後に着手
func (c *GetDeadLetterJobs) Handle(r *rest.Request) *rest.Response {
	return &rest.Response{
		Code: http.StatusNotImplemented,
		Body: "TODO: Not implemented",
	}
}
