package restkit

import "log/slog"

// ContractViolation is a defect in which an implementation and its own
// declaration disagree: an endpoint answered with a status it never declared,
// a handler never ran the check its declaration promised, and so on. These are
// bugs in the application rather than bad requests, and what they have in
// common is that someone has to be told which endpoint broke which promise.
//
// A violation is built as one value and then used by whichever branch the
// environment selected — a panic where a developer is working, a record and a
// served request in production. Building the branches from separate values is
// how they drift apart: one of them ends up naming the endpoint and the other
// one does not, and understanding why means knowing how the middleware outside
// restkit happens to be configured.
//
// The contract is about naming log fields, not about panicking. A violation
// that is only ever reported with a 500 and a record implements the same
// interface, deliberately: it lets every kind of violation be expanded by a
// single type assertion wherever records are written, so adding a kind does not
// mean touching the place that writes them.
type ContractViolation interface {
	error

	// LogAttrs returns the fields that identify the violation.
	//
	// It must not return a "type" field. The record's type belongs to whoever
	// writes the record, because the same violation is written under different
	// types depending on where it surfaced — LogTypeDeclarationDrift when
	// restkit logs it, "panic_log" when the recovery middleware does. A value
	// that carried its own would put the same key in one JSON record twice, and
	// an alert condition matching on it would have no way to say which of the
	// two occurrences it meant.
	LogAttrs() []slog.Attr
}

// LogArgs adapts a violation's fields to the variadic arguments loggear and
// slog take, so that a caller can place them in a record beside fields of its
// own — flat, or nested under a group where the keys would otherwise collide.
func LogArgs(v ContractViolation) []any {
	attrs := v.LogAttrs()
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	return args
}
