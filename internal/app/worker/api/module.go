package system

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/worker/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/app/worker/api/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/worker"
)

const ModuleName = "default"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

// The module constructors build their endpoints here rather than receiving them
// through dependency injection. A typed endpoint is identified by its request
// and response types, and several of these endpoints share the same pair (both
// retry endpoints are Endpoint[NoBody, DeadLetterJobRetry]), which the
// container cannot tell apart. Building them here keeps the wiring unambiguous.

func NewListJobsModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/jobs",
		Get:         restkit.NewController(http.NewListJobs(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewJobStatusModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/jobs/{id}/status",
		Get:         restkit.NewController(http.NewJobStatus(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewCancelJobModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/jobs/{id}",
		Delete:      restkit.NewController(http.NewCancelJob(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewListDeadLetterJobsModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs",
		Get:         restkit.NewController(http.NewListDeadLetterJobs(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewGetDeadLetterJobModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/{id}",
		Get:         restkit.NewController(http.NewGetDeadLetterJobs(wrk)),
		Delete:      restkit.NewController(http.NewDeleteDeadLetterJob(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewRetryDeadLetterJobModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/{id}/retry",
		Post:        restkit.NewController(http.NewRetryDeadLetterJob(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewRetryDeadLetterJobsByNameModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/retry-by-name",
		Post:        restkit.NewController(http.NewRetryDeadLetterJobsByName(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewRetryAllDeadLetterJobsModule(wrk *worker.Worker) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/retry-all",
		Post:        restkit.NewController(http.NewRetryAllDeadLetterJobs(wrk)),
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.AsHTTPModule(NewListJobsModule),
		dikit.AsHTTPModule(NewJobStatusModule),
		dikit.AsHTTPModule(NewCancelJobModule),
		dikit.AsHTTPModule(NewListDeadLetterJobsModule),
		dikit.AsHTTPModule(NewGetDeadLetterJobModule),
		dikit.AsHTTPModule(NewRetryDeadLetterJobModule),
		dikit.AsHTTPModule(NewRetryDeadLetterJobsByNameModule),
		dikit.AsHTTPModule(NewRetryAllDeadLetterJobsModule),
	})
}
