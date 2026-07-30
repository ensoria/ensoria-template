package http_test

import (
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workerhttp "github.com/ensoria/ensoria-template/internal/app/worker/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/worker/pkg/job"
)

// adminCaller holds both worker scopes.
func adminCaller() *authkit.Principal {
	return &authkit.Principal{
		Subject: "svc_ops",
		Scheme:  authkit.SchemeJWT,
		Scopes:  []string{"admin:jobs:read", "admin:jobs:write"},
	}
}

func callerWith(scopes ...string) *authkit.Principal {
	return &authkit.Principal{Subject: "svc_ops", Scheme: authkit.SchemeJWT, Scopes: scopes}
}

// call runs a controller against a request with the given body, caller and id.
func call(ctrl rest.Controller, method, body string, caller *authkit.Principal, id string) *rest.Response {
	raw := httptest.NewRequest(method, "/_/jobs", strings.NewReader(body))
	raw.Header.Set("Content-Type", "application/json")
	if id != "" {
		raw.SetPathValue("id", id)
	}
	r := rest.NewRequest(raw)
	if caller != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), caller))
	}
	return ctrl.Handle(r)
}

func errorCode(res *rest.Response) string {
	GinkgoHelper()
	envelope, ok := res.Body.(*restkit.ErrorEnvelope)
	Expect(ok).To(BeTrue(), "the response body is not the shared error envelope")
	return envelope.Error.Code
}

const noBody = ""

var _ = Describe("worker admin endpoints", func() {
	Describe("who may call them", func() {
		DescribeTable("refuse a caller with no credential",
			func(build func() rest.Controller, method, body string) {
				res := call(build(), method, body, nil, "job_1")

				Expect(res.Code).To(Equal(nethttp.StatusUnauthorized))
				Expect(errorCode(res)).To(Equal(restkit.UnauthenticatedCode))
			},
			Entry("job status", func() rest.Controller {
				return restkit.NewController(workerhttp.NewJobStatus(workerWith(newFakeQueue())))
			}, nethttp.MethodGet, noBody),
			Entry("cancel job", func() rest.Controller {
				return restkit.NewController(workerhttp.NewCancelJob(workerWith(newFakeQueue())))
			}, nethttp.MethodDelete, noBody),
			Entry("list dead letter jobs", func() rest.Controller {
				return restkit.NewController(workerhttp.NewListDeadLetterJobs(workerWith(newFakeQueue())))
			}, nethttp.MethodGet, noBody),
			Entry("retry one", func() rest.Controller {
				return restkit.NewController(workerhttp.NewRetryDeadLetterJob(workerWith(newFakeQueue())))
			}, nethttp.MethodPost, noBody),
			Entry("retry by name", func() rest.Controller {
				return restkit.NewController(workerhttp.NewRetryDeadLetterJobsByName(workerWith(newFakeQueue())))
			}, nethttp.MethodPost, `{"jobName":"send-mail"}`),
			Entry("delete dead letter job", func() rest.Controller {
				return restkit.NewController(workerhttp.NewDeleteDeadLetterJob(workerWith(newFakeQueue())))
			}, nethttp.MethodDelete, noBody),
		)

		// Reading job state does not grant the right to retry or discard.
		DescribeTable("refuse a caller holding only the read scope",
			func(build func() rest.Controller, method, body string) {
				res := call(build(), method, body, callerWith("admin:jobs:read"), "job_1")

				Expect(res.Code).To(Equal(nethttp.StatusForbidden))
				Expect(errorCode(res)).To(Equal(restkit.ForbiddenCode))
			},
			Entry("cancel job", func() rest.Controller {
				return restkit.NewController(workerhttp.NewCancelJob(workerWith(newFakeQueue())))
			}, nethttp.MethodDelete, noBody),
			Entry("retry one", func() rest.Controller {
				return restkit.NewController(workerhttp.NewRetryDeadLetterJob(workerWith(newFakeQueue())))
			}, nethttp.MethodPost, noBody),
			Entry("retry all", func() rest.Controller {
				return restkit.NewController(workerhttp.NewRetryAllDeadLetterJobs(workerWith(newFakeQueue())))
			}, nethttp.MethodPost, noBody),
			Entry("delete dead letter job", func() rest.Controller {
				return restkit.NewController(workerhttp.NewDeleteDeadLetterJob(workerWith(newFakeQueue())))
			}, nethttp.MethodDelete, noBody),
		)

		It("serves a read-scoped caller on a read endpoint", func() {
			q := newFakeQueue().withQueued("job_1")
			ctrl := restkit.NewController(workerhttp.NewJobStatus(workerWith(q)))

			res := call(ctrl, nethttp.MethodGet, noBody, callerWith("admin:jobs:read"), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
		})
	})

	Describe("what they validate", func() {
		It("refuses a job id that is missing", func() {
			ctrl := restkit.NewController(workerhttp.NewJobStatus(workerWith(newFakeQueue())))

			res := call(ctrl, nethttp.MethodGet, noBody, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
		})

		It("refuses a retry-by-name with no job name", func() {
			ctrl := restkit.NewController(workerhttp.NewRetryDeadLetterJobsByName(workerWith(newFakeQueue())))

			res := call(ctrl, nethttp.MethodPost, `{}`, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
			envelope := res.Body.(*restkit.ErrorEnvelope)
			Expect(envelope.Error.FieldErrors[0].Field).To(Equal("jobName"))
		})
	})

	Describe("how they report a worker that cannot answer", func() {
		It("answers 404 for a job the queue does not hold", func() {
			ctrl := restkit.NewController(workerhttp.NewJobStatus(workerWith(newFakeQueue())))

			res := call(ctrl, nethttp.MethodGet, noBody, adminCaller(), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusNotFound))
			Expect(errorCode(res)).To(Equal(workerhttp.CodeJobNotFound))
		})

		// An unrecognised failure must not put the queue's message on the wire.
		It("answers 500 without the underlying message", func() {
			q := newFakeQueue()
			q.failWith = errors.New("redis: connection refused")
			ctrl := restkit.NewController(workerhttp.NewListDeadLetterJobs(workerWith(q)))

			res := call(ctrl, nethttp.MethodGet, noBody, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusInternalServerError))
			Expect(errorCode(res)).NotTo(ContainSubstring("redis"))
		})
	})

	// Each endpoint must reach its own worker call. Wiring one endpoint to
	// another's action is invisible at compile time.
	Describe("what they do", func() {
		It("reports the status of a queued job", func() {
			q := newFakeQueue().withQueued("job_1")
			ctrl := restkit.NewController(workerhttp.NewJobStatus(workerWith(q)))

			res := call(ctrl, nethttp.MethodGet, noBody, adminCaller(), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(res.Body).To(HaveField("Status", Equal(string(job.StatusQueued))))
		})

		It("cancels a queued job and answers with no content", func() {
			q := newFakeQueue().withQueued("job_1")
			ctrl := restkit.NewController(workerhttp.NewCancelJob(workerWith(q)))

			res := call(ctrl, nethttp.MethodDelete, noBody, adminCaller(), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusNoContent))
			Expect(q.queued).NotTo(HaveKey("job_1"))
		})

		It("lists the dead letter jobs with their count", func() {
			q := newFakeQueue().withDeadLetter("job_1", "send-mail").withDeadLetter("job_2", "send-mail")
			ctrl := restkit.NewController(workerhttp.NewListDeadLetterJobs(workerWith(q)))

			res := call(ctrl, nethttp.MethodGet, noBody, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(res.Body).To(HaveField("Count", Equal(2)))
		})

		// Retrying takes the job out of the dead letter queue and puts it back
		// on the queue; discarding only takes it out.
		It("retries one job by moving it back onto the queue", func() {
			q := newFakeQueue().withDeadLetter("job_1", "send-mail")
			ctrl := restkit.NewController(workerhttp.NewRetryDeadLetterJob(workerWith(q)))

			res := call(ctrl, nethttp.MethodPost, noBody, adminCaller(), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(q.deadLetter).NotTo(HaveKey("job_1"))
			Expect(q.queued).To(HaveKey("job_1"))
		})

		It("discards a job without putting it back on the queue", func() {
			q := newFakeQueue().withDeadLetter("job_1", "send-mail")
			ctrl := restkit.NewController(workerhttp.NewDeleteDeadLetterJob(workerWith(q)))

			res := call(ctrl, nethttp.MethodDelete, noBody, adminCaller(), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(q.deadLetter).NotTo(HaveKey("job_1"))
			Expect(q.queued).To(BeEmpty())
		})

		It("retries only the jobs registered under the given name", func() {
			q := newFakeQueue().withDeadLetter("job_1", "send-mail").withDeadLetter("job_2", "build-report")
			ctrl := restkit.NewController(workerhttp.NewRetryDeadLetterJobsByName(workerWith(q)))

			res := call(ctrl, nethttp.MethodPost, `{"jobName":"send-mail"}`, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(res.Body).To(HaveField("RetryCount", Equal(1)))
			Expect(q.deadLetter).To(HaveKey("job_2"))
		})

		It("retries every dead letter job", func() {
			q := newFakeQueue().withDeadLetter("job_1", "send-mail").withDeadLetter("job_2", "build-report")
			ctrl := restkit.NewController(workerhttp.NewRetryAllDeadLetterJobs(workerWith(q)))

			res := call(ctrl, nethttp.MethodPost, noBody, adminCaller(), "")

			Expect(res.Code).To(Equal(nethttp.StatusOK))
			Expect(res.Body).To(HaveField("RetryCount", Equal(2)))
			Expect(q.deadLetter).To(BeEmpty())
		})
	})

	// These endpoints exist so the route is reserved and documented. They must
	// say so rather than fail in a way that looks like the job is missing.
	DescribeTable("endpoints whose worker feature does not exist yet answer 501",
		func(build func() rest.Controller) {
			res := call(build(), nethttp.MethodGet, noBody, adminCaller(), "job_1")

			Expect(res.Code).To(Equal(nethttp.StatusNotImplemented))
			Expect(errorCode(res)).To(Equal(workerhttp.CodeNotImplemented))
		},
		Entry("list jobs", func() rest.Controller {
			return restkit.NewController(workerhttp.NewListJobs(workerWith(newFakeQueue())))
		}),
		Entry("fetch one dead letter job", func() rest.Controller {
			return restkit.NewController(workerhttp.NewGetDeadLetterJobs(workerWith(newFakeQueue())))
		}),
	)
})
