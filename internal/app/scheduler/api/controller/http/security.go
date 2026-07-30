package http

// Scopes required by the scheduler admin endpoints.
//
// Reading task state and changing it are separate scopes so that an operator
// dashboard can be granted the first without the second.
//
// These endpoints also sit behind the SysAdminOnly middleware, which decides
// which clients may reach them at all. The scopes here decide what a caller
// that did reach them is allowed to do.
const (
	ScopeTasksRead  = "admin:tasks:read"
	ScopeTasksWrite = "admin:tasks:write"
)

// Error codes returned by the scheduler admin endpoints.
const (
	// CodeTaskNotFound is returned when no task is registered under the name.
	CodeTaskNotFound = "task_not_found"
	// CodeTaskControlFailed is returned when the scheduler refused the change.
	CodeTaskControlFailed = "task_control_failed"
)
