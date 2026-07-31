package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/scheduler"
	"github.com/ensoria/validator/pkg/rule"
)

// The four task-control endpoints differ only in the scheduler call they make
// and in whether they take a reason, so they are built from one shared shape.

// nameRules validates the `name` path value. Declaring it here means the
// adapter answers 422 with field errors before Handle runs; no endpoint has to
// check for a missing task name itself.
func nameRules() []*rule.RuleSet {
	return []*rule.RuleSet{
		{Field: "name", Rules: []rule.Rule{vkit.Required()}},
	}
}

// reasonRules requires the reason recorded alongside a pause or a disable —
// the next operator to look at the task needs to know why it was stopped.
func reasonRules() []*rule.RuleSet {
	return []*rule.RuleSet{
		{Field: "reason", Rules: []rule.Rule{vkit.Required()}},
	}
}

// controlErrors is the error set every task-control endpoint can answer with.
func controlErrors() []restkit.ErrorSpec {
	return []restkit.ErrorSpec{
		{
			Status:       http.StatusConflict,
			Code:         CodeTaskControlFailed,
			Condition:    "The scheduler refused the change (no such task, or the task is already in that state)",
			CallerAction: "Read the current state with GET /_/tasks/{name} before retrying.",
		},
	}
}

// newControl builds one task-control endpoint.
//
// action performs the change and receives the task name taken from the path.
// bodyRules is nil for the endpoints that take no request body.
func newControl[Req any](
	summary, task, effect, done string,
	bodyRules []*rule.RuleSet,
	action func(ctx context.Context, name string, req *Req) error,
) *restkit.Endpoint[Req, dto.TaskControl] {
	return &restkit.Endpoint[Req, dto.TaskControl]{
		Summary:   summary,
		Task:      task,
		Success:   http.StatusOK,
		Security:  &restkit.SecuritySpec{Scopes: []string{ScopeTasksWrite}},
		PathRules: nameRules(),
		BodyRules: bodyRules,
		// All four are idempotent: applying the same state twice leaves the
		// task as it already is.
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{effect},
			Idempotent:  new(true),
		},
		Errors: controlErrors(),
		Handle: func(r *rest.Request, body *Req) (*rest.Result[dto.TaskControl], error) {
			name, _ := r.PathValue("name")
			if err := action(r.Context(), name, body); err != nil {
				// The scheduler does not distinguish "no such task" from
				// "cannot do that now", so both are reported as a conflict and
				// the message carries the detail.
				return nil, restkit.NewError(http.StatusConflict, CodeTaskControlFailed, err.Error())
			}
			return rest.NewResult(&dto.TaskControl{
				Message: fmt.Sprintf("task [%s] %s", name, done),
			}), nil
		},
	}
}

// NewPauseTask pauses a task until it is resumed (typed Endpoint).
func NewPauseTask(sch *scheduler.Scheduler) *restkit.Endpoint[dto.PauseTask, dto.TaskControl] {
	return newControl(
		"Pause a scheduled task",
		"pause a scheduled task",
		"stops the task from running until it is resumed",
		"paused",
		reasonRules(),
		func(ctx context.Context, name string, req *dto.PauseTask) error {
			return sch.PauseTask(ctx, name, req.Reason)
		},
	)
}

// NewResumeTask resumes a paused task (typed Endpoint).
func NewResumeTask(sch *scheduler.Scheduler) *restkit.Endpoint[restkit.NoBody, dto.TaskControl] {
	return newControl(
		"Resume a paused task",
		"resume a scheduled task",
		"lets the task run on its schedule again",
		"resumed",
		nil,
		func(ctx context.Context, name string, _ *restkit.NoBody) error {
			return sch.ResumeTask(ctx, name)
		},
	)
}

// NewDisableTask disables a task until it is enabled again (typed Endpoint).
func NewDisableTask(sch *scheduler.Scheduler) *restkit.Endpoint[dto.DisableTask, dto.TaskControl] {
	return newControl(
		"Disable a scheduled task",
		"disable a scheduled task",
		"stops the task from running until it is enabled again",
		"disabled",
		reasonRules(),
		func(ctx context.Context, name string, req *dto.DisableTask) error {
			return sch.DisableTask(ctx, name, req.Reason)
		},
	)
}

// NewEnableTask enables a disabled task (typed Endpoint).
func NewEnableTask(sch *scheduler.Scheduler) *restkit.Endpoint[restkit.NoBody, dto.TaskControl] {
	return newControl(
		"Enable a disabled task",
		"enable a scheduled task",
		"lets the task run on its schedule again",
		"enabled",
		nil,
		func(ctx context.Context, name string, _ *restkit.NoBody) error {
			return sch.EnableTask(ctx, name)
		},
	)
}
