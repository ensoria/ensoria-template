package system

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/scheduler"
)

const ModuleName = "default"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

// TODO: add healthcheck endpoint

// The module constructors build their endpoints here rather than receiving them
// through dependency injection. A typed endpoint is identified by its request
// and response types, and several of these endpoints share the same pair
// (Resume and Enable are both Endpoint[NoBody, TaskControl]), which the
// container cannot tell apart. Building them here keeps the wiring unambiguous.

func NewListTasksModule(sch *scheduler.Scheduler) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks",
		Get:         restkit.NewController(http.NewListTasks(sch)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewTaskStateModule(sch *scheduler.Scheduler) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}",
		Get:         restkit.NewController(http.NewGetStatus(sch)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewPauseTaskModule(sch *scheduler.Scheduler) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/pause",
		Post:        restkit.NewController(http.NewPauseTask(sch)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewResumeTaskModule(sch *scheduler.Scheduler) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/resume",
		Post:        restkit.NewController(http.NewResumeTask(sch)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewDisableTaskModule(sch *scheduler.Scheduler) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/disable",
		Post:        restkit.NewController(http.NewDisableTask(sch)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewEnableTaskModule(sch *scheduler.Scheduler) *rest.Module {
	return &rest.Module{
		Path:        "/_/tasks/{name}/enable",
		Post:        restkit.NewController(http.NewEnableTask(sch)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.AsHTTPModule(NewListTasksModule),
		dikit.AsHTTPModule(NewTaskStateModule),
		dikit.AsHTTPModule(NewPauseTaskModule),
		dikit.AsHTTPModule(NewResumeTaskModule),
		dikit.AsHTTPModule(NewDisableTaskModule),
		dikit.AsHTTPModule(NewEnableTaskModule),
	})
}
