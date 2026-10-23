package worker

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/worker/pkg/history"
	"github.com/ensoria/worker/pkg/job"
	"github.com/ensoria/worker/pkg/queue"
	libworker "github.com/ensoria/worker/pkg/worker"
)

// jobTimeout bounds how long a spec waits for the worker to run a job.
const jobTimeout = 10 * time.Second

// memoryQueue is a job queue held in memory: what is enqueued is dequeued, in
// order, and every other operation succeeds without doing anything. It lets a
// real worker run real jobs without a Redis.
type memoryQueue struct {
	*queue.NoopQueue
	jobs chan *job.JobData
}

func newMemoryQueue() *memoryQueue {
	return &memoryQueue{NoopQueue: &queue.NoopQueue{}, jobs: make(chan *job.JobData, 8)}
}

func (q *memoryQueue) Enqueue(_ context.Context, jd *job.JobData) error {
	q.jobs <- jd
	return nil
}

func (q *memoryQueue) Dequeue(ctx context.Context, timeout time.Duration) (*job.JobData, error) {
	select {
	case jd := <-q.jobs:
		return jd, nil
	case <-time.After(timeout):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

const reportJob = "report"

var _ = Describe("a job", func() {
	// run enqueues one job from enqueueCtx on a worker built with the
	// application's options, and returns who the handler saw starting it.
	run := func(claims *initiator.ClaimSet, enqueueCtx context.Context) *initiator.Initiator {
		GinkgoHelper()

		opts := append(workerOptions(claims), libworker.WithHistory(&history.NoopRepository{}))
		w := libworker.New(newMemoryQueue(), opts...)
		seen := make(chan *initiator.Initiator, 1)
		w.Register(reportJob, func(ctx context.Context, _ []byte) error {
			seen <- initiator.From(ctx)
			return nil
		})

		runCtx, stop := context.WithCancel(context.Background())
		DeferCleanup(stop)
		go w.Start(runCtx)

		_, err := w.Enqueue(enqueueCtx, reportJob, map[string]string{})
		Expect(err).NotTo(HaveOccurred())

		var from *initiator.Initiator
		Eventually(seen).WithTimeout(jobTimeout).Should(Receive(&from))
		return from
	}

	It("enqueued while serving a request runs as the caller who made it", func() {
		set, err := initiator.NewClaimSet([]string{"tenant"})
		Expect(err).NotTo(HaveOccurred())
		request := authkit.WithPrincipal(initiator.WithEntry(context.Background(), set), &authkit.Principal{
			Subject: "user-1", Scheme: authkit.SchemeSession, Claims: authkit.Claims{"tenant": "t-1", "email": "x@example.com"},
		})

		// The request's cancellation is detached, as Enqueue asks; its values
		// are what carries who started the work.
		from := run(set, context.WithoutCancel(request))

		Expect(from.Kind()).To(Equal(initiator.KindAppUser))
		Expect(from.ID()).To(Equal("user-1"))
		Expect(from.Claims()).To(Equal(authkit.Claims{"tenant": "t-1"}))
	})

	It("enqueued by a scheduled task runs as the system", func() {
		from := run(nil, initiator.WithSystem(context.Background()))

		Expect(from.Kind()).To(Equal(initiator.KindSystem))
	})

	// A context without values — context.Background() — carries nothing, so
	// the job cannot know who started it.
	It("enqueued with a context that carries nothing runs as unknown", func() {
		from := run(nil, context.Background())

		Expect(from.Kind()).To(Equal(initiator.KindUnknown))
	})
})
