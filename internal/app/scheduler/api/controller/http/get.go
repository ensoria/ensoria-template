package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/dto"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/scheduler"
)

type ListTasks struct {
	scheduler *scheduler.Scheduler
}

func NewListTasks(scheduler *scheduler.Scheduler) *ListTasks {
	return &ListTasks{
		scheduler: scheduler,
	}
}

func (c *ListTasks) Handle(r *rest.Request) *rest.Response {
	ctx := r.Context()
	statuses, err := c.scheduler.ListTaskStates(ctx)
	if err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.TaskControlError{Message: err.Error()},
		}
	}

	// 元のcontrol.TaskStateはsnake_caseなのでdtoにマッピングする
	respBody := make([]dto.TaskStateResponse, 0, len(statuses))
	for _, state := range statuses {
		respBody = append(respBody, dto.TaskStateResponse{
			TaskName:   state.TaskName,
			Status:     string(state.Status),
			PausedAt:   state.PausedAt,
			DisabledAt: state.DisabledAt,
			Reason:     state.Reason,
			UpdatedAt:  state.UpdatedAt,
		})
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: respBody,
	}
}

type GetStatus struct {
	scheduler *scheduler.Scheduler
}

func NewGetStatus(scheduler *scheduler.Scheduler) *GetStatus {
	return &GetStatus{
		scheduler: scheduler,
	}
}

func (c *GetStatus) Handle(r *rest.Request) *rest.Response {
	taskName, exists := r.PathValue("name")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.TaskControlError{Message: "task name is required"},
		}
	}

	ctx := r.Context()
	state, err := c.scheduler.GetTaskState(ctx, taskName)
	if err != nil {
		return &rest.Response{
			Code: http.StatusNotFound,
			Body: &dto.TaskControlError{Message: err.Error()},
		}
	}

	// 元のcontrol.TaskStateはsnake_caseなのでdtoにマッピングする
	respBody := &dto.TaskStateResponse{
		TaskName:   state.TaskName,
		Status:     string(state.Status),
		PausedAt:   state.PausedAt,
		DisabledAt: state.DisabledAt,
		Reason:     state.Reason,
		UpdatedAt:  state.UpdatedAt,
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: respBody,
	}
}
