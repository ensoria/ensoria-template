package http

// Scopes required by the scheduler admin endpoints.
//
// The names read <component>.<resource>:<action>. An application's own scopes
// read <resource>:<action> ("orders:read"), and "tasks" is a common enough word
// that an application may well have a module of that name; prefixing the
// component that owns the resource keeps "may read the application's tasks" and
// "may read the scheduler's tasks" from ever being the same permission, while the
// action still follows the one colon every other scope has.
//
// Reading task state and changing it are separate scopes so that an operator
// dashboard can be granted the first without the second.
//
// These endpoints also sit behind the SysAdminOnly middleware, which decides
// which clients may reach them at all. The scopes here decide what a caller
// that did reach them is allowed to do.
const (
	ScopeTasksRead  = "scheduler.tasks:read"
	ScopeTasksWrite = "scheduler.tasks:write"
)

// Error codes returned by the scheduler admin endpoints.
const (
	// CodeTaskNotFound is returned when no task is registered under the name.
	CodeTaskNotFound = "task_not_found"
	// CodeTaskControlFailed is returned when the scheduler refused the change.
	CodeTaskControlFailed = "task_control_failed"
)
