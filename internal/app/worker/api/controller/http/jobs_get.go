package http

import (
	"fmt"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/worker/api/dto"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

type ListJobs struct {
	worker *worker.Worker
}

func NewListJobs(worker *worker.Worker) *ListJobs {
	return &ListJobs{
		worker: worker,
	}
}

// TODO: workerで一覧機能が未実装のため、workerでの実行完了後に着手
// TODO: パラメータなどで絞れたり、ソート、ページングができるようにする
func (c *ListJobs) Handle(r *rest.Request) *rest.Response {
	return &rest.Response{
		Code: http.StatusNotFound,
		Body: "TODO: Not implemented",
	}
}

type JobStatus struct {
	worker *worker.Worker
}

func NewJobStatus(worker *worker.Worker) *JobStatus {
	return &JobStatus{
		worker: worker,
	}
}

func (c *JobStatus) Handle(r *rest.Request) *rest.Response {
	jobId, exists := r.PathValue("id")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.JobControlError{Message: "job id is required"},
		}
	}

	ctx := r.Context()
	status, err := c.worker.GetJobStatus(ctx, jobId)
	if err != nil {
		return &rest.Response{
			Code: http.StatusNotFound,
			Body: &dto.JobControlError{Message: fmt.Sprintf("job not found: %s", err.Error())},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.JobStatus{
			Id:     jobId,
			Status: string(status),
		},
	}
}
