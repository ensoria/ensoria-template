package describe

import (
	"errors"
	"reflect"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/file/pkg/file"
	"github.com/ensoria/mb/pkg/mb"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	"github.com/ensoria/scheduler/pkg/scheduler"
	workerDB "github.com/ensoria/worker/pkg/database"
	"github.com/ensoria/worker/pkg/worker"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

// requireEveryStub asks for every type stubs() provides.
//
// It is written out by hand so that fx actually builds each one — a stub that
// compiles but cannot be constructed (a changed library signature, a storage
// registry that rejects its own default) fails here rather than in whatever
// generator run happens to reach it first. The spec below checks this parameter
// list against stubs(), so the two cannot drift apart.
func requireEveryStub(
	_ dikit.RootContext,
	_ enscache.Cache,
	_ file.Storage,
	_ file.FileSystem,
	_ workerDB.DatabaseClient,
	_ schedulerDB.DatabaseClient,
	_ authkit.Verifier,
	_ *goredis.Client,
	_ mb.Publish,
	_ worker.Enqueuer,
	_ mb.Subscriber,
	_ mb.Publisher,
	_ mb.StartSubscription,
	_ *scheduler.Scheduler,
	_ *worker.Worker,
) {
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// providedTypes reads the type each entry in stubs() yields.
func providedTypes() []reflect.Type {
	var types []reflect.Type
	for i, provider := range stubs() {
		t := reflect.TypeOf(provider)
		ExpectWithOffset(1, t.Kind()).To(Equal(reflect.Func),
			"stubs()[%d] is not a plain constructor; teach this helper to read its result type", i)

		for out := 0; out < t.NumOut(); out++ {
			if t.Out(out) == errorType {
				continue
			}
			types = append(types, t.Out(out))
		}
	}
	return types
}

// requiredTypes reads the parameter list of requireEveryStub.
func requiredTypes() []reflect.Type {
	t := reflect.TypeOf(requireEveryStub)
	types := make([]reflect.Type, 0, t.NumIn())
	for in := 0; in < t.NumIn(); in++ {
		types = append(types, t.In(in))
	}
	return types
}

var _ = Describe("stubs", func() {
	// Resolution alone would not prove much: fx builds lazily, so a stub nothing
	// asks for is never constructed, and seven of these are currently asked for
	// by nothing at all. Requiring them is what makes the check real.
	It("builds every type it provides", func() {
		app := fx.New(
			fx.Provide(stubs()...),
			fx.Invoke(requireEveryStub),
			fx.NopLogger,
		)

		Expect(app.Err()).NotTo(HaveOccurred())
	})

	// Without this, adding a stub and forgetting to require it would leave the
	// new one untested while every spec stayed green.
	It("provides exactly the types the spec above requires", func() {
		Expect(providedTypes()).To(ConsistOf(requiredTypes()))
	})
})

var _ = Describe("withStubHint", func() {
	// The whole point is that fx buries the type name, so the hint has to survive
	// being at the end of a long chain rather than matching a tidy message.
	It("names the missing type and where to add it", func() {
		err := withStubHint(errors.New(
			`could not build arguments for function "reflect".makeFuncStub: ` +
				`missing dependencies for function "reflect".makeFuncStub: missing type: cache.Cache`))

		Expect(err.Error()).To(ContainSubstring("describe has no stub for: cache.Cache"))
		Expect(err.Error()).To(ContainSubstring(stubsFile))
	})

	It("keeps the original error wrapped", func() {
		original := errors.New("missing type: cache.Cache")

		err := withStubHint(original)

		Expect(errors.Is(err, original)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(original.Error()))
	})

	// A single failure can reach several missing types, and reporting only the
	// first would send the reader round the loop once per type.
	It("lists every missing type", func() {
		err := withStubHint(errors.New("missing type: cache.Cache and missing type: file.Storage"))

		Expect(err.Error()).To(ContainSubstring("describe has no stub for: cache.Cache, file.Storage"))
	})

	It("reports a type reached twice only once", func() {
		err := withStubHint(errors.New("missing type: cache.Cache ... missing type: cache.Cache"))

		Expect(err.Error()).To(ContainSubstring("describe has no stub for: cache.Cache\n"))
	})

	// fx's wording is not a contract. If it changes, the guidance disappears and
	// the error still says everything it said before.
	It("returns the error untouched when no type name can be read", func() {
		original := errors.New("some other failure entirely")

		Expect(withStubHint(original)).To(BeIdenticalTo(original))
	})
})
