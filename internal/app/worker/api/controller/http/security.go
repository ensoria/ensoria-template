package http

// Scopes required by the worker admin endpoints.
//
// Reading job state and changing it are separate scopes so that an operator
// dashboard can be granted the first without the second.
//
// These endpoints also sit behind the SysAdminOnly middleware, which decides
// which clients may reach them at all. The scopes here decide what a caller
// that did reach them is allowed to do.
const (
	ScopeJobsRead  = "admin:jobs:read"
	ScopeJobsWrite = "admin:jobs:write"
)

// Error codes returned by the worker admin endpoints.
const (
	// CodeJobNotFound is returned when no job is stored under the id.
	CodeJobNotFound = "job_not_found"
	// CodeJobControlFailed is returned when the worker refused the change.
	CodeJobControlFailed = "job_control_failed"
	// CodeNotImplemented marks an endpoint whose backing worker feature does
	// not exist yet. It is declared rather than returned as a bare body so the
	// gap shows up in the generated documentation.
	CodeNotImplemented = "not_implemented"
)
