package describe

import (
	"context"
	"database/sql"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/cache/pkg/cachememory"
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

// Names that only ever exist inside describe. They are not read from the
// configuration: describe has to resolve under every environment, including one
// where no cache or storage is configured at all.
const (
	// stubCacheKeyPrefix namespaces the keys of the in-memory application cache.
	stubCacheKeyPrefix = "describe"
	// stubDiskName is the single disk the stub Storage registers, and the one it
	// exposes as its default.
	stubDiskName = "memory"
)

// stubs returns the providers describe puts underneath the module constructors.
//
// Every type listed here is one the running application provides (server.Run or
// scheduler.Start) and that a module can therefore inject. describe registers no
// real infrastructure constructor, so a type a module reaches but this list does
// not carry makes the whole resolution fail with `missing type: X` — see the
// README's "Adding a dependency that describe has to stub".
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

		// Application cache. The library's own in-memory implementation, so
		// there is no fake to keep in step with the interface.
		func() enscache.Cache { return cachememory.New(stubCacheKeyPrefix) },

		// File storage. Storage and FileSystem come from the same in-memory
		// disk, the way the application derives one from the other.
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

		// The management endpoints take these to declare their endpoints, and
		// describe does not run a handler. Building the real ones would need
		// Redis and a database, so a zero value is what they get.
		func() *scheduler.Scheduler { return &scheduler.Scheduler{} },
		func() *worker.Worker { return &worker.Worker{} },
	}
}

// stubStorage builds the storage registry describe injects: a single in-memory
// disk, registered as the default.
func stubStorage() (file.Storage, error) {
	return file.NewStorage(
		file.WithDisk(stubDiskName, filememory.New()),
		file.WithDefault(stubDiskName),
	)
}

// stubFileSystem exposes the stub Storage's default disk as file.FileSystem,
// mirroring storage.NewDefaultFileSystem. Deriving it keeps a module that
// injects both from seeing two different instances of the same abstraction.
//
// Default() cannot be nil here: file.NewStorage rejects a default that names no
// registered disk, so stubStorage would have failed first.
func stubFileSystem(storage file.Storage) file.FileSystem {
	return storage.Default()
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
