package system

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/rest/pkg/rest"
)

const ModuleName = "default"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

// TODO: add healthcheck endpoint

func NewListTasksModule(listTasks *http.ListTasks) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks",
		Get:         listTasks,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewTaskStateModule(getStatus *http.GetStatus) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}",
		Get:         getStatus,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewPauseTaskModule(pauseTask *http.ResumeTask) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/pause",
		Post:        pauseTask,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewResumeTaskModule(resumeTask *http.ResumeTask) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/resume",
		Post:        resumeTask,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewDisableTaskModule(disableTask *http.DisableTask) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/disable",
		Post:        disableTask,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewEnableTaskModule(enableTask *http.EnableTask) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/enable",
		Post:        enableTask,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func init() {
	dikit.AppendConstructors([]any{
		http.NewListTasks,
		dikit.AsHTTPModule(NewListTasksModule),

		http.NewGetStatus,
		dikit.AsHTTPModule(NewTaskStateModule),

		http.NewPauseTask,
		dikit.AsHTTPModule(NewPauseTaskModule),

		http.NewResumeTask,
		dikit.AsHTTPModule(NewResumeTaskModule),

		http.NewDisableTask,
		dikit.AsHTTPModule(NewDisableTaskModule),

		http.NewEnableTask,
		dikit.AsHTTPModule(NewEnableTaskModule),
	})
}
