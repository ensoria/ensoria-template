package scheduler

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
)

// The task is what RegisterTasks hands the scheduler, so these specs exercise
// the wrapping RegisterTasks applies, without a scheduler backend.
var _ = Describe("a registered task", func() {
	It("runs as the system", func() {
		var seen *initiator.Initiator
		task := asSystemTask(func(ctx context.Context) error {
			seen = initiator.From(ctx)
			return nil
		})

		Expect(task(context.Background())).To(Succeed())
		Expect(seen.Kind()).To(Equal(initiator.KindSystem))
	})

	It("keeps the context the scheduler gave it", func() {
		type key struct{}
		var seen any
		task := asSystemTask(func(ctx context.Context) error {
			seen = ctx.Value(key{})
			return nil
		})

		Expect(task(context.WithValue(context.Background(), key{}, "from the scheduler"))).To(Succeed())
		Expect(seen).To(Equal("from the scheduler"))
	})

	It("returns what the task returns", func() {
		failure := errors.New("task failed")
		task := asSystemTask(func(context.Context) error { return failure })

		Expect(task(context.Background())).To(MatchError(failure))
	})
})
