package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"go.uber.org/fx"
	ggrpc "google.golang.org/grpc"
)

// fakeLifecycle stands in for fx's lifecycle so that the hooks can be run
// outside an application.
type fakeLifecycle struct {
	hooks []fx.Hook
}

func (l *fakeLifecycle) Append(h fx.Hook) { l.hooks = append(l.hooks, h) }

// dikit.LC is what the registration takes; the fake has to satisfy it.
var _ dikit.LC = (*fakeLifecycle)(nil)

// fakeShutdowner records the shutdown the serving goroutine asks for when the
// server stops on its own.
type fakeShutdowner struct{}

func (*fakeShutdowner) Shutdown(...fx.ShutdownOption) error { return nil }

// freePort asks the operating system for a port nothing is using, so that the
// specs below can assert which port was listened on without picking a number
// and hoping.
func freePort() int {
	GinkgoHelper()

	listener, err := net.Listen("tcp", ":0")
	Expect(err).NotTo(HaveOccurred())
	port := listener.Addr().(*net.TCPAddr).Port
	Expect(listener.Close()).To(Succeed())

	return port
}

var _ = Describe("reflectionDefaultForEnv", func() {
	// Reflection publishes the service definitions, so the default has to be
	// "no" anywhere they are not already public.
	It("serves reflection in the environments a developer runs against", func() {
		for _, name := range []string{"local", "development"} {
			Expect(reflectionDefaultForEnv(&name)).To(BeTrue(), "env %s", name)
		}
	})

	It("does not serve it anywhere else", func() {
		for _, name := range []string{"test", "staging", "production", "", "unknown"} {
			Expect(reflectionDefaultForEnv(&name)).To(BeFalse(), "env %s", name)
		}
	})

	It("does not serve it when no environment was named", func() {
		Expect(reflectionDefaultForEnv(nil)).To(BeFalse())
	})

	// The configuration is what has the last word, in both directions: a
	// staging deployment being debugged turns it on, and a development
	// deployment reachable from outside turns it off.
	It("is only the fallback: the configuration overrules it either way", func() {
		local := "local"
		production := "production"
		on, off := true, false

		Expect((&appconfig.GRPC{Reflection: &off}).ReflectionEnabled(reflectionDefaultForEnv(&local))).To(BeFalse())
		Expect((&appconfig.GRPC{Reflection: &on}).ReflectionEnabled(reflectionDefaultForEnv(&production))).To(BeTrue())
	})
})

var _ = Describe("serverLimits", func() {
	// An option nobody configured is left out rather than passed as a zero, so
	// that grpc-go's own defaults stay in place.
	It("passes no option when nothing is configured", func() {
		Expect(serverLimits(&appconfig.GRPC{})).To(BeEmpty())
	})

	It("passes one option per configured message size", func() {
		Expect(serverLimits(&appconfig.GRPC{MaxRecvMsgSize: 1})).To(HaveLen(1))
		Expect(serverLimits(&appconfig.GRPC{MaxSendMsgSize: 1})).To(HaveLen(1))
		Expect(serverLimits(&appconfig.GRPC{MaxRecvMsgSize: 1, MaxSendMsgSize: 1})).To(HaveLen(2))
	})

	// The two keepalive halves are separate options, and each is passed as soon
	// as anything in it is set: grpc-go fills the remaining zero fields with
	// its own defaults, so a half-filled struct is safe.
	It("passes the keepalive parameters when either of them is configured", func() {
		Expect(serverLimits(&appconfig.GRPC{KeepaliveTime: time.Second})).To(HaveLen(1))
		Expect(serverLimits(&appconfig.GRPC{KeepaliveTimeout: time.Second})).To(HaveLen(1))
		Expect(serverLimits(&appconfig.GRPC{KeepaliveTime: time.Second, KeepaliveTimeout: time.Second})).To(HaveLen(1))
	})

	It("passes the enforcement policy when either of them is configured", func() {
		Expect(serverLimits(&appconfig.GRPC{KeepaliveMinTime: time.Second})).To(HaveLen(1))
		Expect(serverLimits(&appconfig.GRPC{KeepalivePermitWithoutStream: true})).To(HaveLen(1))
	})

	It("passes both keepalive options when both halves are configured", func() {
		Expect(serverLimits(&appconfig.GRPC{
			KeepaliveTime:    time.Second,
			KeepaliveMinTime: time.Second,
		})).To(HaveLen(2))
	})
})

var _ = Describe("RegisterGRPCServerLifecycle", func() {
	var (
		lifecycle *fakeLifecycle
		server    *ggrpc.Server
		port      int
	)

	BeforeEach(func() {
		lifecycle = &fakeLifecycle{}
		server = ggrpc.NewServer()
		port = freePort()
	})

	start := func(config *appconfig.GRPC) {
		GinkgoHelper()

		RegisterGRPCServerLifecycle(lifecycle, &fakeShutdowner{}, server, config)
		Expect(lifecycle.hooks).To(HaveLen(1))
		Expect(lifecycle.hooks[0].OnStart(context.Background())).To(Succeed())
	}

	stop := func() {
		GinkgoHelper()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Expect(lifecycle.hooks[0].OnStop(ctx)).To(Succeed())
	}

	// C4-D6: the listener and the log line are built from one value, so the
	// port cannot be changed in one place and left behind in the other. What is
	// observable from here is the listener; a second literal would show up as a
	// server listening somewhere other than the configured port.
	It("listens on the configured port", func() {
		start(&appconfig.GRPC{Port: port})
		defer stop()

		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(conn.Close()).To(Succeed())
	})

	It("reports a port it cannot bind instead of serving on another one", func() {
		blocker, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(blocker.Close()).To(Succeed()) }()

		RegisterGRPCServerLifecycle(lifecycle, &fakeShutdowner{}, server, &appconfig.GRPC{Port: port})

		Expect(lifecycle.hooks[0].OnStart(context.Background())).
			To(MatchError(ContainSubstring("gRPC server failed to listen")))
	})

	// The configured grace period is applied inside the lifecycle's deadline,
	// not beside it, so a shutdown still finishes either way.
	It("stops with a graceful stop timeout configured", func() {
		start(&appconfig.GRPC{Port: port, GracefulStopTimeout: time.Second})

		stop()
	})

	It("registers nothing for a nil server", func() {
		RegisterGRPCServerLifecycle(lifecycle, &fakeShutdowner{}, nil, &appconfig.GRPC{Port: port})

		Expect(lifecycle.hooks).To(BeEmpty())
	})
})
