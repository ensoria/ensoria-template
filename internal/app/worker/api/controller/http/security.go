package http

// Scopes required by the worker admin endpoints.
//
// The names read <component>.<resource>:<action>. An application's own scopes
// read <resource>:<action> ("orders:read"), and "jobs" is a common enough word
// that an application may well have a module of that name; prefixing the
// component that owns the resource keeps "may read the application's jobs" and
// "may read the worker's jobs" from ever being the same permission, while the
// action still follows the one colon every other scope has.
//
// Reading job state and changing it are separate scopes so that an operator
// dashboard can be granted the first without the second.
//
// These endpoints also sit behind the SysAdminOnly middleware, which decides
// which clients may reach them at all. The scopes here decide what a caller
// that did reach them is allowed to do.
const (
	ScopeJobsRead  = "worker.jobs:read"
	ScopeJobsWrite = "worker.jobs:write"
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
