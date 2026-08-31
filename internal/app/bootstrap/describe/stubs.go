package describe

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/config/pkg/registry"
	infrastorage "github.com/ensoria/ensoria-template/internal/infra/storage"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/file/pkg/file"
	"github.com/ensoria/file/pkg/filememory"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/rest/pkg/rest"
	schedulerDB "github.com/ensoria/scheduler/pkg/database"
	"github.com/ensoria/scheduler/pkg/scheduler"
	workerDB "github.com/ensoria/worker/pkg/database"
	"github.com/ensoria/worker/pkg/job"
	"github.com/ensoria/worker/pkg/worker"
	goredis "github.com/redis/go-redis/v9"
)

// stubCacheKeyPrefix namespaces the keys of the in-memory application cache. It
// is fixed rather than read from the configuration, because describe has to
// resolve under every environment, including one with no cache configured.
const stubCacheKeyPrefix = "describe"

// stubsFile is where the hint on a resolution failure sends the reader. It is
// this file; the path is written out because the person reading the message is
// looking at a generator's output, not at this package.
const stubsFile = "internal/app/bootstrap/describe/stubs.go"

// missingTypePattern reads the type names out of fx's dependency report.
var missingTypePattern = regexp.MustCompile(`missing type:\s*(\S+)`)

// withStubHint appends the fix to a resolution failure.
//
// fx reports a missing provider as a chain that names reflect internals and ends,
// many lines in, with `missing type: X`. Whoever hits it is usually not thinking
// about describe at all — they added a dependency to a module and ran the
// document generator — so the type names are lifted out of the wall of text into
// a sentence that says what to do with them.
//
// The original error is wrapped, never replaced: the hint is an addition. When no
// type name can be read out (fx reworded its report, or the failure was something
// else entirely) err is returned exactly as it came, so a pattern that stops
// matching costs the guidance and nothing else.
func withStubHint(err error) error {
	matches := missingTypePattern.FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return err
	}

	// One failure can name several types, and can name the same type twice when
	// two branches of the graph both reach it. Listing it twice tells the reader
	// nothing, so each name is reported once, in the order fx reached it.
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		name := match[1]
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	return fmt.Errorf("%w\n\ndescribe has no stub for: %s\nAdd one for each to %s",
		err, strings.Join(names, ", "), stubsFile)
}

// stubs returns the providers describe puts underneath the module constructors.
//
// What belongs here is decided by one mechanical rule: a type is stubbed when
// server.Run or scheduler.Start provides it and a module could inject it. Not
// "does a module inject it today" — that answer changes every time a module
// gains a dependency, which is exactly how describe broke before: fx builds
// lazily, so a type nobody reached was a hole nobody could see. Stubbing by what
// is injectable keeps the decision independent of what the modules currently do.
//
// The only types the application provides that are left out are *pipeline.HTTP
// and *wsrouter.Router. That is structural rather than a preference: both
// consume the module groups (fx.ParamTags(GroupTagHttpModules) on
// CreateHTTPPipeline), so a module injecting one would be a dependency cycle in
// the running application too.
//
// A type a module reaches but this list does not carry makes the whole
// resolution fail with `missing type: X`; withStubHint turns that into a line
// saying which type to add here. The README says the same thing to whoever hits
// it from the other end, under "Adding a dependency that describe has to stub".
//
// Both resolvers take this one list. They used to keep separate hand-written
// lists, which drifted apart and left each of them with holes the other did not
// have.
//
// The stubs are never executed: describe reads declarations and does not run a
// handler, a job or a subscription, and it never starts the fx lifecycle. What
// matters is only that each type can be built.
func stubs() []any {
	return []any{
		// Not infrastructure: RootContext holds no connection and starts no
		// goroutine. It only registers an OnStop hook, which never runs because
		// describe does not start the lifecycle, so the real constructor is
		// registered instead of a stub.
		dikit.ProvideRootContext,

		// The resolved configuration. Modules read it through the registry
		// package's own functions rather than by injection, so nothing asks for
		// this today — but server.Run and scheduler.Start both provide it, so a
		// module can, and the rule above does not ask whether one currently
		// does.
		//
		// It is the real registry rather than a stub, for the same reason
		// RootContext is: BuildHTTP and BuildMessaging fill it with
		// InitializeConfiguration before resolving anything, and what a
		// declaration says can depend on what was configured. A fake here would
		// describe an application nobody is running.
		registry.DefaultRegistry,

		// Application cache. The library's own in-memory implementation, so
		// there is no fake to keep in step with the interface.
		func() enscache.Cache { return cachememory.New(stubCacheKeyPrefix) },

		// File storage. The same disk names the application registers, backed by
		// memory, with FileSystem derived from the default disk the way the
		// application derives it.
		stubStorage,
		stubFileSystem,

		// The history databases the worker and the scheduler write to.
		func() workerDB.DatabaseClient { return &stubWorkerDBClient{} },
		func() schedulerDB.DatabaseClient { return &stubSchedulerDBClient{} },

		// Request authentication.
		func() authkit.Verifier { return &stubVerifier{} },

		// The raw queue handle. server.Run provides it unnamed, so a module can
		// inject it; go-redis connects lazily, so this client never dials.
		func() *goredis.Client { return goredis.NewClient(&goredis.Options{}) },

		// Broker publishing and job enqueueing.
		func() mb.Publish { return stubPublish },
		func() worker.Enqueuer { return &stubEnqueuer{} },

		// The broker connections and the subscribe entry point. Modules reach
		// the broker through mb.Publish and declare subscriptions with mbkit, so
		// nothing injects these today — but the application provides all three
		// unnamed, so a module can, and the rule above does not ask whether one
		// currently does.
		func() mb.Subscriber { return &stubSubscriber{} },
		func() mb.Publisher { return &stubPublisher{} },
		func() mb.StartSubscription { return stubStartSubscription },

		// The management endpoints take these to declare their endpoints, and
		// describe does not run a handler. Building the real ones would need
		// Redis and a database, so a zero value is what they get.
		func() *scheduler.Scheduler { return &scheduler.Scheduler{} },
		func() *worker.Worker { return &worker.Worker{} },
	}
}

// stubStorage builds the storage registry describe injects: the same disks the
// application registers, under the same names and with the same default, each
// one backed by memory instead of a directory or a bucket.
//
// The names are the exported constants rather than copies, because Disk(name) is
// most of what a Storage is for. A registry that resolves the type but rejects
// every name the application accepts would break a constructor that resolves its
// disk up front — and it would break it in document generation only, saying no
// more than `unknown disk "s3"`.
//
// Each disk gets its own filememory instance: local and s3 are separate
// backends in the application, so writing through one is not meant to be visible
// through the other.
//
// TODO: revisit this once storage moves to the config package. The disk names
// are constants today, which is what makes mirroring them a compile-time link.
// Once they come from configuration they become per-environment, and a stub that
// resolves under every environment can no longer name them — it will have to
// derive the registry from the same configuration, or stop promising that
// Disk(name) resolves at all.
func stubStorage() (file.Storage, error) {
	return file.NewStorage(
		file.WithDisk(infrastorage.DiskLocal, filememory.New()),
		file.WithDisk(infrastorage.DiskS3, filememory.New()),
		file.WithDefault(infrastorage.DefaultDisk),
	)
}

// stubFileSystem exposes the stub Storage's default disk as file.FileSystem,
// mirroring storage.NewDefaultFileSystem. Deriving it keeps a module that
// injects both from seeing two different instances of the same abstraction.
//
// Default() cannot be nil here: file.NewStorage rejects a default that names no
// registered disk, so stubStorage would have failed first.
func stubFileSystem(st file.Storage) file.FileSystem {
	return st.Default()
}

// stubWorkerDBClient is a worker history client that owns no connection.
//
// DB returns nil rather than an open handle: nothing describe resolves runs a
// query, and a real *sql.DB would mean a real database.
type stubWorkerDBClient struct{}

func (*stubWorkerDBClient) DB() *sql.DB                    { return nil }
func (*stubWorkerDBClient) Close() error                   { return nil }
func (*stubWorkerDBClient) Ping(ctx context.Context) error { return nil }
func (*stubWorkerDBClient) Type() workerDB.DBType          { return "" }

// stubSchedulerDBClient is the same client for the scheduler's history. It is
// written out a second time because the two libraries declare their own DBType,
// so one implementation cannot satisfy both interfaces.
type stubSchedulerDBClient struct{}

func (*stubSchedulerDBClient) DB() *sql.DB                    { return nil }
func (*stubSchedulerDBClient) Close() error                   { return nil }
func (*stubSchedulerDBClient) Ping(ctx context.Context) error { return nil }
func (*stubSchedulerDBClient) Type() schedulerDB.DBType       { return "" }

// stubVerifier authenticates nobody.
//
// Verify answers as it would for a request that carried no credential, which is
// the one outcome that needs no configuration. Schemes returns none, so nothing
// reads a security scheme off the verifier; the schemes in the generated
// document come from the configuration (see securitySchemes).
type stubVerifier struct{}

func (*stubVerifier) Verify(r *rest.Request) (*authkit.Principal, error) {
	return nil, authkit.ErrNoCredential
}

func (*stubVerifier) Schemes() []string { return nil }

// stubPublish accepts every message and sends none.
var stubPublish mb.Publish = func(ctx context.Context, target string, data []byte, metadata map[string]string, opts ...mb.PublishOption) error {
	return nil
}

// stubEnqueuer accepts every job and queues none.
type stubEnqueuer struct{}

func (*stubEnqueuer) Enqueue(ctx context.Context, jobName string, payload any, opts ...*job.Option) (string, error) {
	return "", nil
}

// stubStartSubscription accepts every subscription and starts none.
var stubStartSubscription mb.StartSubscription = func(target string, handler mb.SubscribeHandler, opts ...mb.SubscribeOption) error {
	return nil
}

// stubSubscriber is a broker subscriber that never dials.
//
// Connect succeeds without a broker, which is the whole point: mb dials
// explicitly rather than lazily, so the real connection would need a running
// broker even to be built.
type stubSubscriber struct{}

func (*stubSubscriber) Connect(ctx context.Context) error { return nil }
func (s *stubSubscriber) SetOptions(opts ...mb.SubscribeOption) mb.Subscriber {
	return s
}

func (*stubSubscriber) Subscribe(ctx context.Context, target string, handler mb.MessageHandler, opts ...mb.SubscribeOption) error {
	return nil
}
func (*stubSubscriber) Ping(ctx context.Context) error { return nil }
func (*stubSubscriber) Close() error                   { return nil }

// stubPublisher is the publishing half of the same non-connection.
type stubPublisher struct{}

func (*stubPublisher) Connect(ctx context.Context) error { return nil }
func (p *stubPublisher) SetOptions(opts ...mb.PublishOption) mb.Publisher {
	return p
}

func (*stubPublisher) Publish(ctx context.Context, target string, data []byte, metadata map[string]string, opts ...mb.PublishOption) error {
	return nil
}
func (*stubPublisher) Ping(ctx context.Context) error { return nil }
func (*stubPublisher) Close() error                   { return nil }
