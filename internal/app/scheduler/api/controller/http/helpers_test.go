package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/control"
	"github.com/ensoria/scheduler/pkg/scheduler"
)

// stateRepo is an in-memory control.StateRepository, so the endpoints run
// against a real scheduler controller without Redis.
type stateRepo struct {
	states map[string]*control.TaskState
	// failWith, when set, is returned by every call. It stands in for a
	// scheduler that cannot reach its backend.
	failWith error
}

func newStateRepo(names ...string) *stateRepo {
	repo := &stateRepo{states: map[string]*control.TaskState{}}
	for _, name := range names {
		repo.states[name] = &control.TaskState{TaskName: name, Status: control.StatusEnabled}
	}
	return repo
}

func (r *stateRepo) SaveState(_ context.Context, state *control.TaskState) error {
	if r.failWith != nil {
		return r.failWith
	}
	r.states[state.TaskName] = state
	return nil
}

func (r *stateRepo) GetState(_ context.Context, name string) (*control.TaskState, error) {
	if r.failWith != nil {
		return nil, r.failWith
	}
	state, ok := r.states[name]
	if !ok {
		return nil, errors.New("task state not found: " + name)
	}
	return state, nil
}

func (r *stateRepo) DeleteState(_ context.Context, name string) error {
	if r.failWith != nil {
		return r.failWith
	}
	delete(r.states, name)
	return nil
}

func (r *stateRepo) ListStates(_ context.Context) ([]*control.TaskState, error) {
	if r.failWith != nil {
		return nil, r.failWith
	}
	states := make([]*control.TaskState, 0, len(r.states))
	for _, s := range r.states {
		states = append(states, s)
	}
	return states, nil
}

// schedulerWith builds a scheduler whose task control is backed by repo.
// The distributed backend is nil because these endpoints never run a task.
//
// WithLogger comes first: WithControl builds the controller with whatever
// logger the scheduler holds at that moment, and the controller logs on every
// change.
func schedulerWith(repo control.StateRepository) *scheduler.Scheduler {
	return scheduler.New(nil,
		scheduler.WithLogger(loggear.GetLogger()),
		scheduler.WithControl(repo),
	)
}

// adminCaller holds both scheduler scopes.
func adminCaller() *authkit.Principal {
	return &authkit.Principal{
		Subject: "svc_ops",
		Scheme:  authkit.SchemeJWT,
		Scopes:  []string{"admin:tasks:read", "admin:tasks:write"},
	}
}

// callerWith holds exactly the given scopes.
func callerWith(scopes ...string) *authkit.Principal {
	return &authkit.Principal{Subject: "svc_ops", Scheme: authkit.SchemeJWT, Scopes: scopes}
}

// request builds a request with the given path values, body and caller.
// A nil caller means nobody is authenticated.
func request(method, body string, caller *authkit.Principal, pathValues map[string]string) *rest.Request {
	raw := httptest.NewRequest(method, "/_/tasks", strings.NewReader(body))
	raw.Header.Set("Content-Type", "application/json")
	for k, v := range pathValues {
		raw.SetPathValue(k, v)
	}
	r := rest.NewRequest(raw)
	if caller != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), caller))
	}
	return r
}

// errorCode reads the code out of the shared error envelope.
func errorCode(res *rest.Response) string {
	GinkgoHelper()
	envelope, ok := res.Body.(*restkit.ErrorEnvelope)
	Expect(ok).To(BeTrue(), "the response body is not the shared error envelope")
	return envelope.Error.Code
}

// fieldErrors reads the field-level errors out of the shared error envelope.
func fieldErrors(res *rest.Response) []restkit.FieldErrorDetail {
	GinkgoHelper()
	envelope, ok := res.Body.(*restkit.ErrorEnvelope)
	Expect(ok).To(BeTrue(), "the response body is not the shared error envelope")
	return envelope.Error.FieldErrors
}

// noBody is the request body of an endpoint that takes none.
const noBody = ""

var _ = http.StatusOK // keep net/http imported for the specs in this package
