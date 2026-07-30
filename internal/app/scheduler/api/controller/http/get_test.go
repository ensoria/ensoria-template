package http_test

import (
	"errors"
	nethttp "net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/dto"
	schedhttp "github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

var _ = Describe("task inspection endpoints", func() {
	const taskName = "daily-report"

	var repo *stateRepo

	BeforeEach(func() { repo = newStateRepo(taskName, "weekly-digest") })

	Describe("GET /_/tasks", func() {
		list := func(caller *authkit.Principal) *rest.Response {
			ctrl := restkit.NewController(schedhttp.NewListTasks(schedulerWith(repo)))
			return ctrl.Handle(request(nethttp.MethodGet, noBody, caller, nil))
		}

		It("refuses a caller with no credential", func() {
			Expect(list(nil).Code).To(Equal(nethttp.StatusUnauthorized))
		})

		// Reading is a separate scope from changing, so the read scope alone
		// has to be enough here.
		It("serves a caller holding only the read scope", func() {
			res := list(callerWith("admin:tasks:read"))

			Expect(res.Code).To(Equal(nethttp.StatusOK))
		})

		It("returns one entry per registered task", func() {
			res := list(adminCaller())

			body, ok := res.Body.(*[]dto.TaskStateResponse)
			Expect(ok).To(BeTrue())
			Expect(*body).To(HaveLen(2))
		})

		// An unrecognised failure must not put the backend's message on the wire.
		It("answers 500 without the underlying message when the backend fails", func() {
			repo.failWith = errors.New("redis: connection refused")

			res := list(adminCaller())

			Expect(res.Code).To(Equal(nethttp.StatusInternalServerError))
			Expect(res.Body).NotTo(HaveField("Error.Message", ContainSubstring("redis")))
		})
	})

	Describe("GET /_/tasks/{name}", func() {
		get := func(caller *authkit.Principal, name string) *rest.Response {
			ctrl := restkit.NewController(schedhttp.NewGetStatus(schedulerWith(repo)))
			return ctrl.Handle(request(nethttp.MethodGet, noBody, caller, map[string]string{"name": name}))
		}

		It("returns the state of the named task", func() {
			res := get(adminCaller(), taskName)

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(res.Body).To(HaveField("TaskName", Equal(taskName)))
		})

		It("answers 404 for a task that is not registered", func() {
			res := get(adminCaller(), "no-such-task")

			Expect(res.Code).To(Equal(nethttp.StatusNotFound))
			Expect(errorCode(res)).To(Equal(schedhttp.CodeTaskNotFound))
		})

		It("refuses a call with no task name", func() {
			res := get(adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
			Expect(fieldErrors(res)[0].Field).To(Equal("name"))
		})
	})
})
