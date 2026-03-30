package system

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/worker/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/app/worker/api/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/rest/pkg/rest"
)

const ModuleName = "default"

func Params() (*appconfig.Parameters, error) {
	return registry.ModuleParams(ModuleName)
}

func NewListJobsModule(get *http.ListJobs) *rest.Module {
	return &rest.Module{
		Path:        "/_/jobs",
		Get:         get,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewJobStatusModule(get *http.JobStatus) *rest.Module {
	return &rest.Module{
		Path:        "/_/jobs/{id}/status",
		Get:         get,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewCancelJobModule(cancel *http.CancelJob) *rest.Module {
	return &rest.Module{
		Path:        "/_/jobs/{id}",
		Delete:      cancel,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewListDeadLetterJobsModule(list *http.ListDeadLetterJobs) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs",
		Get:         list,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewGetDeadLetterJobModule(
	get *http.GetDeadLetterJobs,
	delete *http.DeleteDeadLetterJob,
) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/{id}",
		Get:         get,
		Delete:      delete,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewRetryDeadLetterJobModule(retry *http.RetryDeadLetterJob) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/{id}/retry",
		Post:        retry,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewRetryDeadLetterJobsByNameModule(retryByName *http.RetryDeadLetterJobsByName) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/retry-by-name",
		Post:        retryByName,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func NewRetryAllDeadLetterJobsModule(retryAll *http.RetryAllDeadLetterJobs) *rest.Module {
	return &rest.Module{
		Path:        "/_/dead-letter-jobs/retry-all",
		Post:        retryAll,
		Middlewares: []rest.Middleware{middleware.SysAdminOnly},
	}
}

func init() {
	dikit.AppendConstructors([]any{
		http.NewListJobs,
		dikit.AsHTTPModule(NewListJobsModule),

		http.NewJobStatus,
		dikit.AsHTTPModule(NewJobStatusModule),

		http.NewCancelJob,
		dikit.AsHTTPModule(NewCancelJobModule),

		http.NewListDeadLetterJobs,
		dikit.AsHTTPModule(NewListDeadLetterJobsModule),

		http.NewGetDeadLetterJobs,
		http.NewDeleteDeadLetterJob,
		dikit.AsHTTPModule(NewGetDeadLetterJobModule),

		http.NewRetryDeadLetterJob,
		dikit.AsHTTPModule(NewRetryDeadLetterJobModule),

		http.NewRetryDeadLetterJobsByName,
		dikit.AsHTTPModule(NewRetryDeadLetterJobsByNameModule),

		http.NewRetryAllDeadLetterJobs,
		dikit.AsHTTPModule(NewRetryAllDeadLetterJobsModule),
	})
}
