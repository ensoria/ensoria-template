package main

import (
	"errors"
	"os"

	"github.com/ensoria/ensoria-template/internal/app/bootstrap/scheduler"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/spf13/pflag"
)

func main() {
	// FIXME: configのenvを使って、ここのリストを修正する
	// envList := slices.Join(env.StringList, ", ")
	envVal := pflag.StringP("env", "e", "local", "it must be either [local], [development], [staging], [production] or [test].")
	pflag.Parse()

	if err := scheduler.Start(envVal); err != nil {
		// Every way the application can end other than a clean shutdown arrives
		// here: a graph that failed to build, an OnStart hook that failed, a
		// shutdown that failed, and an exit code the application asked for.
		//
		// The record is structured rather than the standard library's
		// unstructured stderr line, because an abnormal exit is the first thing
		// an operator looks for in the log platform.
		//
		// It goes through the global logger rather than one built here, so that
		// whatever the application configures — a level, a handler, common fields
		// such as a service name — reaches this record too. This is precisely the
		// record that must not fall outside the log setup.
		//
		// There is no chicken-and-egg problem in depending on it: loggear lazy-
		// inits the global logger to a default JSON logger when nothing has
		// configured one, which is exactly the state a failed startup is in.

		// An exit code the application asked for (Shutdowner.Shutdown(ExitCode(n)))
		// is not a failure of this process's own making, so it is not an error
		// record: whoever requested the code is expected to have logged why. The
		// info record keeps the invariant that a non-zero exit always leaves a log
		// line, even when a caller breaks that expectation. It has to be tested
		// before the error record below, or an intentional exit would be logged as
		// an abnormal one.
		var exitErr *dikit.ExitError
		if errors.As(err, &exitErr) {
			loggear.Info("scheduler exited with requested exit code", "code", exitErr.Code)
			os.Exit(exitErr.Code)
		}

		loggear.Error("scheduler exited abnormally", "error", err)
		os.Exit(1)
	}
}
