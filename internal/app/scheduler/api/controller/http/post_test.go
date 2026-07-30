package http_test

import (
	"errors"
	nethttp "net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	schedhttp "github.com/ensoria/ensoria-template/internal/app/scheduler/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/scheduler/pkg/control"
)

var _ = Describe("task control endpoints", func() {
	const taskName = "daily-report"

	var repo *stateRepo

	BeforeEach(func() { repo = newStateRepo(taskName) })

	// pause stands in for all four control endpoints: they are built from the
	// same shape, so what holds here holds for resume, disable and enable.
	pause := func(body string, caller *authkit.Principal, name string) *rest.Response {
		ctrl := restkit.NewController(schedhttp.NewPauseTask(schedulerWith(repo)))
		return ctrl.Handle(request(nethttp.MethodPost, body, caller, map[string]string{"name": name}))
	}

	Describe("who may call it", func() {
		It("refuses a caller with no credential", func() {
			res := pause(`{"reason":"maintenance"}`, nil, taskName)

			Expect(res.Code).To(Equal(nethttp.StatusUnauthorized))
			Expect(errorCode(res)).To(Equal(restkit.UnauthenticatedCode))
		})

		// Reading task state does not grant the right to change it.
		It("refuses a caller holding only the read scope", func() {
			res := pause(`{"reason":"maintenance"}`, callerWith("admin:tasks:read"), taskName)

			Expect(res.Code).To(Equal(nethttp.StatusForbidden))
			Expect(errorCode(res)).To(Equal(restkit.ForbiddenCode))
		})

		It("serves a caller holding the write scope", func() {
			res := pause(`{"reason":"maintenance"}`, adminCaller(), taskName)

			Expect(res.Code).To(Equal(nethttp.StatusOK))
		})
	})

	Describe("what it validates", func() {
		// The reason is what tells the next operator why the task was stopped,
		// so a pause without one is refused.
		It("refuses a pause with no reason", func() {
			res := pause(`{}`, adminCaller(), taskName)

			Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
			Expect(fieldErrors(res)).To(HaveLen(1))
			Expect(fieldErrors(res)[0].Field).To(Equal("reason"))
		})

		It("refuses a call with no task name", func() {
			res := pause(`{"reason":"maintenance"}`, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
			Expect(fieldErrors(res)[0].Field).To(Equal("name"))
		})

		// Refusing before validation keeps field names and constraints away
		// from a caller who has not identified themselves.
		It("refuses an anonymous caller before looking at the body", func() {
			res := pause(`{}`, nil, taskName)

			Expect(res.Code).To(Equal(nethttp.StatusUnauthorized))
		})
	})

	Describe("what it does", func() {
		It("records the pause and the reason", func() {
			pause(`{"reason":"maintenance"}`, adminCaller(), taskName)

			state, err := repo.GetState(nil, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Status).To(Equal(control.StatusPaused))
			Expect(state.Reason).To(Equal("maintenance"))
		})

		It("names the task in the confirmation", func() {
			res := pause(`{"reason":"maintenance"}`, adminCaller(), taskName)

			Expect(res.Body).To(HaveField("Message", ContainSubstring(taskName)))
		})
	})

	Describe("when the scheduler refuses the change", func() {
		BeforeEach(func() { repo.failWith = errors.New("backend unreachable") })

		// The scheduler does not distinguish "no such task" from "cannot do
		// that now", so both are reported as a conflict.
		It("answers 409 rather than 500", func() {
			res := pause(`{"reason":"maintenance"}`, adminCaller(), taskName)

			Expect(res.Code).To(Equal(nethttp.StatusConflict))
			Expect(errorCode(res)).To(Equal(schedhttp.CodeTaskControlFailed))
		})
	})

	// Each endpoint must reach its own scheduler call. Wiring one endpoint to
	// another's action is invisible at compile time — it happened once already,
	// with pause wired to resume.
	// The starting status differs per entry because the scheduler refuses some
	// transitions outright (a paused task must be resumed, not enabled).
	DescribeTable("each endpoint applies its own change",
		func(build func() rest.Controller, body string, from, want control.TaskStatus) {
			repo.states[taskName] = &control.TaskState{TaskName: taskName, Status: from}

			res := build().Handle(request(nethttp.MethodPost, body, adminCaller(),
				map[string]string{"name": taskName}))
			Expect(res.Code).To(Equal(nethttp.StatusOK))

			state, err := repo.GetState(nil, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Status).To(Equal(want))
		},
		Entry("pause", func() rest.Controller {
			return restkit.NewController(schedhttp.NewPauseTask(schedulerWith(repo)))
		}, `{"reason":"maintenance"}`, control.StatusEnabled, control.StatusPaused),
		Entry("resume", func() rest.Controller {
			return restkit.NewController(schedhttp.NewResumeTask(schedulerWith(repo)))
		}, noBody, control.StatusPaused, control.StatusEnabled),
		Entry("disable", func() rest.Controller {
			return restkit.NewController(schedhttp.NewDisableTask(schedulerWith(repo)))
		}, `{"reason":"retired"}`, control.StatusEnabled, control.StatusDisabled),
		Entry("enable", func() rest.Controller {
			return restkit.NewController(schedhttp.NewEnableTask(schedulerWith(repo)))
		}, noBody, control.StatusDisabled, control.StatusEnabled),
	)
})
