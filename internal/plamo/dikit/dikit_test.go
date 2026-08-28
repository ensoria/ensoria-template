package dikit_test

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
)

const (
	// quiet is the outputFxLog value most specs run with. It is the production
	// setting, and the one under which fx used to discard the reason a startup
	// failed: what these specs assert is that the reason reaches the caller
	// regardless of the event logger.
	quiet = false

	// verbose is the debug setting, where fx writes its own event log.
	verbose = true

	// runTimeout bounds a spec rather than the application: every path but an
	// outright failure returns only once the application is asked to shut down,
	// so a regression there would otherwise hang the suite.
	runTimeout = 5 * time.Second

	// requestedExitCode is the code the specs ask for through the Shutdowner.
	requestedExitCode = 3

	// stoppingLine is how fx's console logger renders the Stopping event: the
	// name of the signal that caused the shutdown, uppercased. Shutdowner.Shutdown
	// reports itself as SIGTERM, so a clean stop through it reads "TERMINATED".
	stoppingLine = "TERMINATED"
)

var (
	errConstructor = errors.New("constructor failed")
	errOnStart     = errors.New("OnStart failed")
	errOnStop      = errors.New("OnStop failed")
)

// dependency is what a constructor under test produces. A constructor runs only
// when something depends on it, so the specs pair it with an invocation that
// takes it.
type dependency struct{}

// runApp executes ProvideAndRun off the spec's goroutine and returns its error.
func runApp(constructors []any, invocations []any) error {
	return runAppWithFxLog(constructors, invocations, quiet)
}

func runAppWithFxLog(constructors []any, invocations []any, outputFxLog bool) error {
	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		err = dikit.ProvideAndRun(constructors, invocations, outputFxLog)
	}()
	EventuallyWithOffset(1, done, runTimeout).Should(BeClosed())
	return err
}

// captureStderr collects what fx's console logger writes while f runs. The
// logger is built with os.Stderr inside ProvideAndRun, so the swap has to be in
// place before the call.
func captureStderr(f func()) string {
	reader, writer, err := os.Pipe()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	original := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = original }()

	f()

	ExpectWithOffset(1, writer.Close()).To(Succeed())
	written, err := io.ReadAll(reader)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(written)
}

// shutdownAfter returns an invocation that asks the application to stop as soon
// as it is built. fx caches the last shutdown signal, so a Wait that is only
// reached afterwards still receives it.
func shutdownAfter(opts ...fx.ShutdownOption) any {
	return func(shutdowner dikit.Shutdowner) error {
		return shutdowner.Shutdown(opts...)
	}
}

var _ = Describe("ProvideAndRun", func() {
	Context("when a constructor fails", func() {
		It("returns the error", func() {
			err := runApp(
				[]any{func() (*dependency, error) { return nil, errConstructor }},
				[]any{func(*dependency) {}},
			)
			Expect(err).To(MatchError(ContainSubstring(errConstructor.Error())))
		})
	})

	Context("when an invocation fails", func() {
		It("returns the error", func() {
			err := runApp(nil, []any{func() error { return errConstructor }})
			Expect(err).To(MatchError(ContainSubstring(errConstructor.Error())))
		})
	})

	Context("when an OnStart hook fails", func() {
		It("returns the error", func() {
			err := runApp(nil, []any{func(lc dikit.LC) {
				lc.Append(dikit.Hook{
					OnStart: func(context.Context) error { return errOnStart },
				})
			}})
			Expect(err).To(MatchError(ContainSubstring(errOnStart.Error())))
		})
	})

	Context("when the application shuts down cleanly", func() {
		It("returns nil", func() {
			Expect(runApp(nil, []any{shutdownAfter()})).To(Succeed())
		})
	})

	Context("when the application requests a non-zero exit code", func() {
		It("returns an ExitError carrying the code", func() {
			err := runApp(nil, []any{shutdownAfter(fx.ExitCode(requestedExitCode))})

			var exitErr *dikit.ExitError
			Expect(errors.As(err, &exitErr)).To(BeTrue(), "expected an *ExitError, got %v", err)
			Expect(exitErr.Code).To(Equal(requestedExitCode))
		})
	})

	Context("when fx's own event log is on", func() {
		// Run() emits the Stopping event itself, so ProvideAndRun has to emit it
		// too: without it the debug output would lose a line that the fx-native
		// startup path prints.
		It("emits the Stopping event on the way down", func() {
			output := captureStderr(func() {
				Expect(runAppWithFxLog(nil, []any{shutdownAfter()}, verbose)).To(Succeed())
			})
			Expect(output).To(ContainSubstring(stoppingLine))
		})
	})

	Context("when an OnStop hook fails", func() {
		failingOnStop := func(lc dikit.LC) {
			lc.Append(dikit.Hook{
				OnStop: func(context.Context) error { return errOnStop },
			})
		}

		It("returns the shutdown error", func() {
			err := runApp(nil, []any{failingOnStop, shutdownAfter()})
			Expect(err).To(MatchError(ContainSubstring(errOnStop.Error())))
		})

		It("reports the shutdown failure rather than the requested exit code", func() {
			err := runApp(nil, []any{failingOnStop, shutdownAfter(fx.ExitCode(requestedExitCode))})

			var exitErr *dikit.ExitError
			Expect(errors.As(err, &exitErr)).To(BeFalse(), "a failed shutdown outranks a requested exit code")
			Expect(err).To(MatchError(ContainSubstring(errOnStop.Error())))
		})
	})
})
