package http

import (
	"fmt"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/scheduler"
	"github.com/ensoria/validator/pkg/rule"
)

// REFACTOR: 全体的に重複コードが多いので整理する

type PauseTask struct {
	scheduler *scheduler.Scheduler
}

func NewPauseTask(scheduler *scheduler.Scheduler) *PauseTask {
	return &PauseTask{
		scheduler: scheduler,
	}
}

func (c *PauseTask) Handle(r *rest.Request) *rest.Response {
	taskName, exists := r.PathValue("name")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.TaskControlError{Message: "task name is required"},
		}
	}

	pt, msgs := vkit.RestRequestBody[dto.PauseTask](
		r,
		&rule.RuleSet{Field: "reason", Rules: []rule.Rule{vkit.Required()}},
	)
	if msgs != nil {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: msgs,
		}
	}

	ctx := r.Context()
	if err := c.scheduler.PauseTask(ctx, taskName, pt.Reason); err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.TaskControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.TaskControl{Message: fmt.Sprintf("task [%s] paused", taskName)},
	}

}

type ResumeTask struct {
	scheduler *scheduler.Scheduler
}

func NewResumeTask(scheduler *scheduler.Scheduler) *ResumeTask {
	return &ResumeTask{
		scheduler: scheduler,
	}
}

func (c *ResumeTask) Handle(r *rest.Request) *rest.Response {
	taskName, exists := r.PathValue("name")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.TaskControlError{Message: "task name is required"},
		}
	}

	ctx := r.Context()
	if err := c.scheduler.ResumeTask(ctx, taskName); err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.TaskControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.TaskControl{Message: fmt.Sprintf("task [%s] resumed", taskName)},
	}
}

type DisableTask struct {
	scheduler *scheduler.Scheduler
}

func NewDisableTask(scheduler *scheduler.Scheduler) *DisableTask {
	return &DisableTask{
		scheduler: scheduler,
	}
}

func (c *DisableTask) Handle(r *rest.Request) *rest.Response {
	taskName, exists := r.PathValue("name")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.TaskControlError{Message: "task name is required"},
		}
	}

	dt, msgs := vkit.RestRequestBody[dto.DisableTask](
		r,
		&rule.RuleSet{Field: "reason", Rules: []rule.Rule{vkit.Required()}},
	)
	if msgs != nil {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: msgs,
		}
	}

	ctx := r.Context()
	if err := c.scheduler.DisableTask(ctx, taskName, dt.Reason); err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.TaskControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.TaskControl{Message: fmt.Sprintf("task [%s] disabled", taskName)},
	}

}

type EnableTask struct {
	scheduler *scheduler.Scheduler
}

func NewEnableTask(scheduler *scheduler.Scheduler) *EnableTask {
	return &EnableTask{
		scheduler: scheduler,
	}
}

func (c *EnableTask) Handle(r *rest.Request) *rest.Response {
	taskName, exists := r.PathValue("name")
	if !exists {
		return &rest.Response{
			Code: http.StatusBadRequest,
			Body: &dto.TaskControlError{Message: "task name is required"},
		}
	}

	ctx := r.Context()
	if err := c.scheduler.EnableTask(ctx, taskName); err != nil {
		return &rest.Response{
			Code: http.StatusInternalServerError,
			Body: &dto.TaskControlError{Message: err.Error()},
		}
	}

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.TaskControl{Message: fmt.Sprintf("task [%s] enabled", taskName)},
	}
}
