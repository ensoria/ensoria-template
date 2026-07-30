package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/control"
	"github.com/ensoria/scheduler/pkg/scheduler"
	"github.com/ensoria/validator/pkg/rule"
)

// NewListTasks lists the state of every registered task (typed Endpoint).
// GET takes no body, so the request type is restkit.NoBody.
func NewListTasks(sch *scheduler.Scheduler) *restkit.Endpoint[restkit.NoBody, []dto.TaskStateResponse] {
	return &restkit.Endpoint[restkit.NoBody, []dto.TaskStateResponse]{
		Summary:  "List the state of every scheduled task",
		Task:     "inspect scheduled tasks",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{ScopeTasksRead}},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Related: []string{
			"Inspect one task: GET /_/tasks/{name}",
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[[]dto.TaskStateResponse], error) {
			states, err := sch.ListTaskStates(r.Context())
			if err != nil {
				// An unrecognised error becomes 500 without leaking its text.
				return nil, err
			}

			// scheduler.TaskState is the scheduler's own shape; map it to the DTO.
			body := make([]dto.TaskStateResponse, 0, len(states))
			for _, state := range states {
				body = append(body, taskStateResponse(state))
			}
			return rest.NewResult(&body), nil
		},
	}
}

// NewGetStatus fetches the state of one task by name (typed Endpoint).
//
// The `name` path value is declared in PathRules, so a missing or over-long
// name is rejected by the adapter with 422 + field errors before Handle runs.
func NewGetStatus(sch *scheduler.Scheduler) *restkit.Endpoint[restkit.NoBody, dto.TaskStateResponse] {
	return &restkit.Endpoint[restkit.NoBody, dto.TaskStateResponse]{
		Summary:  "Fetch the state of one scheduled task",
		Task:     "inspect a scheduled task",
		Success:  http.StatusOK,
		Security: &restkit.SecuritySpec{Scopes: []string{ScopeTasksRead}},
		PathRules: []*rule.RuleSet{
			{Field: "name", Rules: []rule.Rule{vkit.Required()}},
		},
		FieldDocs: map[string]string{
			"status": "One of the scheduler task states (running, paused, disabled)",
		},
		Behavior: restkit.BehaviorSpec{
			SideEffects: []string{"none"},
			Idempotent:  new(true),
		},
		Errors: []restkit.ErrorSpec{
			{
				Status:       http.StatusNotFound,
				Code:         CodeTaskNotFound,
				Condition:    "No task is registered under that name",
				CallerAction: "Check the name against GET /_/tasks. Do not retry.",
			},
		},
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.TaskStateResponse], error) {
			name, _ := r.PathValue("name")
			state, err := sch.GetTaskState(r.Context(), name)
			if err != nil {
				return nil, restkit.NewError(http.StatusNotFound, CodeTaskNotFound, err.Error())
			}

			body := taskStateResponse(state)
			return rest.NewResult(&body), nil
		},
	}
}

// taskStateResponse maps the scheduler's own task state onto the response DTO.
// The scheduler uses snake_case field names; the API uses camelCase.
func taskStateResponse(state *control.TaskState) dto.TaskStateResponse {
	return dto.TaskStateResponse{
		TaskName:   state.TaskName,
		Status:     string(state.Status),
		PausedAt:   state.PausedAt,
		DisabledAt: state.DisabledAt,
		Reason:     state.Reason,
		UpdatedAt:  state.UpdatedAt,
	}
}
