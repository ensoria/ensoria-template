package restkit

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// LogTypeAuthorizationDrift labels the record written when an endpoint's
// resource check and its declaration disagree — the declaration promises a
// check the handler never ran, or the handler runs one the declaration does not
// describe.
//
// It is named after declaration_drift_log because it is the same kind of fact:
// what the generated documentation says and what the code does have come apart.
// An alert on one belongs next to an alert on the other, and both mean a defect
// rather than a bad request. See the README's alerting section.
const LogTypeAuthorizationDrift = "authorization_drift_log"

// ResourceCheck declares that an endpoint's decision needs the resource itself.
//
// Scopes answer "may this caller update orders at all"; they cannot answer "is
// this order theirs", because that requires reading the order. So the decision
// becomes a step inside the handler — and a step inside a handler is exactly
// the kind of rule that quietly stops matching the documentation.
//
// This type is what stops that: the declaration holds a reference to the very
// predicate the handler runs, and the adapter refuses to serve a success the
// predicate never approved (see Authorize). The generated documentation is
// built from Description, so a caller reads about the constraint the code
// actually enforces.
//
// Build one with NewResourceCheck; the zero value carries no predicate and is
// not usable.
type ResourceCheck struct {
	// Description states the constraint in one sentence, for a caller to read
	// ("Only the owner of the order can update it."). It is what the generated
	// documentation prints, so it is written for them rather than for the
	// implementer. Required.
	Description string

	// declared is the type the predicate takes. Authorize compares what the
	// handler passes against it, so that a mismatch is a named defect rather
	// than a failed type assertion inside the closure.
	declared reflect.Type

	// check is the predicate with its type parameter closed over, which is what
	// lets a non-generic SecuritySpec hold a typed decision. It may assume the
	// resource is of the declared type; Authorize has already checked.
	check func(p *authkit.Principal, resource any) bool
}

// NewResourceCheck ties a declaration to the predicate that decides it.
//
//	Security: &restkit.SecuritySpec{
//	    Scopes: []string{"orders:write"},
//	    Resource: restkit.NewResourceCheck[dto.Order](
//	        "Only the owner of the order can update it.",
//	        func(p *authkit.Principal, o *dto.Order) bool { return service.IsOwner(p.Subject, o) },
//	    ),
//	}
//
// The type parameter is what makes the declaration type-safe: the predicate is
// written against the resource type it means, and the handler is checked
// against the same type when it calls Authorize.
//
// # Where the predicate belongs
//
// In the domain, not here. What "owner" means is part of the order's
// specification, and an authorization framework has no business defining it.
// Note also that the predicate above takes a subject rather than a Principal:
// keeping authkit out of the domain's signatures means the rule can be read,
// tested and reused without the authorization layer.
//
// ⚠ The predicate is called with exactly what the handler passed, a nil pointer
// included. A handler that can reach Authorize without a resource has usually
// skipped a 404 it should have answered first — but if yours can, say so in the
// predicate (`return o != nil && ...`) rather than dereferencing.
func NewResourceCheck[T any](
	description string, check func(p *authkit.Principal, resource *T) bool,
) *ResourceCheck {
	return &ResourceCheck{
		Description: description,
		declared:    reflect.TypeFor[*T](),
		check: func(p *authkit.Principal, resource any) bool {
			// Safe by construction: Authorize compares the type first.
			return check(p, resource.(*T))
		},
	}
}

// Authorize runs the resource check the endpoint declares, and records that it
// ran.
//
// Call it as soon as the resource exists, and always before answering:
//
//	order, err := svc.FindOrder(id)
//	if err != nil {
//	    return nil, err
//	}
//	if err := restkit.Authorize(r, order); err != nil {
//	    return nil, err
//	}
//
// A refused caller gets 403 in the same shape as every other error, so a
// resource-level refusal is indistinguishable from a scope-level one — which is
// what a caller should see, since neither is something they can retry. A
// request with no caller at all is refused the same way: nobody owns anything.
//
// # What it guarantees
//
// The adapter will not serve a success from an endpoint declaring a check that
// never approved it: forgetting the call above turns into a 500 and a record,
// not into a resource served to whoever asked. See ResourceCheck.
//
// ⚠ The guarantee is about a *buffered* response, which is every response a
// typed Endpoint can produce today. Should streaming ever reach this adapter,
// the rule becomes "Authorize must precede the first write", and the guarantee
// then covers the first byte of the body — not the status line, which the
// pipeline sends before the body and cannot take back. See TODO-streaming-1.md.
//
// # Endpoints with more than one resource
//
// One check per endpoint. Calling Authorize again runs the same check on
// another resource, which is what a handler loading a list wants; but an
// endpoint needing two genuinely different rules is asking to be two endpoints,
// and this deliberately does not help it stay as one.
func Authorize(r *rest.Request, resource any) error {
	state, ok := authorizationStateFrom(r.Context())
	if !ok {
		// Reached from outside a typed endpoint's Handle — a raw
		// rest.Controller, or a handler called directly by something other than
		// the adapter. There is no declaration to answer with, and nothing else
		// will report this, so it is reported here.
		return reportDrift(&UndeclaredAuthorization{
			Method: r.Method(), Path: r.Path(), Passed: typeName(resource),
		})
	}

	if state.check == nil {
		state.reported = true
		return reportDrift(&UndeclaredAuthorization{
			Method: r.Method(), Path: r.Path(), Passed: typeName(resource),
		})
	}
	if passed := reflect.TypeOf(resource); passed != state.check.declared {
		state.reported = true
		return reportDrift(&UndeclaredAuthorization{
			Method:   r.Method(),
			Path:     r.Path(),
			Declared: state.check.declared.String(),
			Passed:   typeName(resource),
		})
	}

	// A running application always has a caller here: authorize() answers 401
	// before the handler runs unless the endpoint is public, and a public
	// endpoint carrying a resource check is refused at startup. The remaining
	// way to arrive without one is to call a handler directly, from a test of
	// that very combination — and refusing is the honest answer even then,
	// since nobody owns anything. It is written out rather than left implicit
	// so that the predicate is never handed a nil caller to dereference.
	principal, ok := authkit.PrincipalFrom(r.Context())
	if !ok || !state.check.check(principal, resource) {
		return NewError(http.StatusForbidden, ForbiddenCode, forbiddenMessage)
	}

	state.done = true
	return nil
}

// MissingAuthorization is the contract violation an endpoint commits by
// answering with a success that the resource check it declares never approved.
//
// It is the defect this whole mechanism exists to catch, and the reason the
// check is on in production as well: a declaration promising that only the
// owner may update an order, with a handler that forgot to ask, is an endpoint
// serving everyone. That is not a documentation problem to be logged and moved
// past — it is the 500 the caller should get.
type MissingAuthorization struct {
	// Method and Path name the endpoint, taken from the request: a route is
	// only a route once a method is attached to it.
	Method string
	Path   string
}

var _ ContractViolation = (*MissingAuthorization)(nil)

// Error says what to do about it, because it is read at the top of a failing
// test rather than in a post-mortem.
func (m *MissingAuthorization) Error() string {
	return "the endpoint declares a resource check that the handler never ran: " +
		"call restkit.Authorize with the resource before returning a success, " +
		"or drop Resource from Endpoint.Security if the constraint is not real"
}

// LogAttrs returns the fields that identify the endpoint. It carries no "type"
// of its own — see ContractViolation.LogAttrs for why.
func (m *MissingAuthorization) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("method", m.Method),
		slog.String("path", m.Path),
	}
}

// UndeclaredAuthorization is the contract violation a handler commits by asking
// for a resource check the endpoint's declaration cannot answer: none is
// declared, or the one declared is for another type.
//
// It is the mirror of MissingAuthorization and a much less dangerous defect —
// the handler is stricter than the document, not laxer. It is still refused
// rather than logged and allowed, because there is no answer Authorize could
// give instead. Returning "allowed" would let a handler serve a resource it
// believes was checked, which is the very thing this mechanism exists to
// prevent; returning "refused" would deny a caller over a wiring mistake.
type UndeclaredAuthorization struct {
	// Method and Path name the endpoint.
	Method string
	Path   string
	// Declared is the resource type the endpoint's check takes, empty when the
	// endpoint declares no check at all.
	Declared string
	// Passed is the type the handler handed to Authorize.
	Passed string
}

var _ ContractViolation = (*UndeclaredAuthorization)(nil)

func (u *UndeclaredAuthorization) Error() string {
	if u.Declared == "" {
		return fmt.Sprintf(
			"the handler ran a resource check on %s that the endpoint does not declare: "+
				"add Resource to Endpoint.Security so the generated documentation "+
				"states the constraint, or stop calling restkit.Authorize", u.Passed)
	}
	return fmt.Sprintf(
		"the endpoint declares a resource check for %s but the handler passed %s: "+
			"hand Authorize the type the declared predicate takes", u.Declared, u.Passed)
}

func (u *UndeclaredAuthorization) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("method", u.Method),
		slog.String("path", u.Path),
		slog.String("passed_resource", u.Passed),
	}
	if u.Declared != "" {
		attrs = append(attrs, slog.String("declared_resource", u.Declared))
	}
	return attrs
}

// authorizationState tracks what the handler did about the resource check its
// endpoint declares.
//
// It is reached through the request context as a pointer, so that Authorize can
// write to the same value the adapter reads afterwards — including when the
// handler has derived a context of its own, which a value in a context could
// not survive.
//
// # Which side writes the record
//
// Two places can write the drift record, and the rule between them is:
// **Authorize reports what it finds; the adapter reports what never happened.**
//
//	violation                              record written by   500 answered by
//	------------------------------------   -----------------   ------------------
//	declared a check, never called          verify              verify
//	called with no declaration / wrong type  Authorize          the ordinary error
//	                                                            path, once the
//	                                                            handler returns it
//	…and the handler ignored that error      nobody (already)   verify
//
// The split is forced rather than chosen, and it is worth saying why, because a
// single reporting point would be the obvious thing to want:
//
//   - It cannot all be Authorize. The first row is the defect of Authorize not
//     having been called, so there is no call in which to notice it.
//   - It cannot all be the adapter. Authorize is reachable from outside a typed
//     endpoint — a raw rest.Controller, or a handler invoked directly — where
//     there is no state and no adapter to notice anything. And on the ordinary
//     path for the second row, the handler returns the error, so Handle answers
//     from errorResponse and never reaches verify.
//
// The last row is the seam between the two, and reported is what holds it: the
// handler was told and carried on regardless, so the answer still has to be
// refused, while the record has already been written and must not appear twice.
type authorizationState struct {
	// check is the endpoint's declaration, nil when it declares none.
	check *ResourceCheck
	// done records that a declared check ran and approved the caller.
	done bool
	// reported records that Authorize already wrote a record. See the rule
	// above for what the adapter does with it.
	reported bool
}

// authorizationKey is unexported, so nothing outside this package can put a
// state on a context or replace the one the adapter placed.
type authorizationKey struct{}

func withAuthorizationState(ctx context.Context, state *authorizationState) context.Context {
	return context.WithValue(ctx, authorizationKey{}, state)
}

func authorizationStateFrom(ctx context.Context) (*authorizationState, bool) {
	state, ok := ctx.Value(authorizationKey{}).(*authorizationState)
	return state, ok && state != nil
}

// verify reports the endpoint if the handler is about to answer with a success
// the declaration does not support. It returns the response to send instead, or
// nil to let the handler's own answer through.
//
// It is the adapter's half of the reporting rule on authorizationState: what
// never happened. It is reached only after the handler returned a success —
// a handler that answered with an error served no resource, so a check it never
// ran is not a defect.
func (s *authorizationState) verify(r *rest.Request) *rest.Response {
	switch {
	case s.reported:
		// Authorize already said what went wrong, and the handler answered with
		// a success regardless. The record is not written twice; the answer is
		// still refused.
		return internalErrorResponse()
	case s.check != nil && !s.done:
		return reportDriftResponse(&MissingAuthorization{Method: r.Method(), Path: r.Path()})
	default:
		return nil
	}
}

// reportDrift writes the record and hands the violation back as the error a
// handler returns.
func reportDrift(v ContractViolation) error {
	loggear.Error(v.Error(),
		append(LogArgs(v), slog.String("type", LogTypeAuthorizationDrift))...)
	return v
}

// reportDriftResponse writes the record and produces the answer to send in
// place of the handler's.
func reportDriftResponse(v ContractViolation) *rest.Response {
	_ = reportDrift(v)
	return internalErrorResponse()
}

// typeName names what a handler passed, including the nil it may have passed by
// mistake.
func typeName(resource any) string {
	t := reflect.TypeOf(resource)
	if t == nil {
		return "nil"
	}
	return t.String()
}

// resourceCheckOf reads the check out of a declaration that may not exist. A
// nil declaration means "a verified caller is required" and nothing more, so it
// declares no resource rule.
func resourceCheckOf(security *SecuritySpec) *ResourceCheck {
	if security == nil {
		return nil
	}
	return security.Resource
}
