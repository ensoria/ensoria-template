package main

import (
	"os"

	"github.com/ensoria/ensoria-template/internal/app/bootstrap/scheduler"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/spf13/pflag"
)

func main() {
	// FIXME: configのenvを使って、ここのリストを修正する
	// envList := slices.Join(env.StringList, ", ")
	envVal := pflag.StringP("env", "e", "local", "it must be either [local], [development], [staging], [production] or [test].")
	pflag.Parse()

	if err := scheduler.Start(envVal); err != nil {
		// A failed startup is the first thing an operator looks for in the log
		// platform, so it is written as a structured record rather than as the
		// standard library's unstructured stderr line.
		//
		// It goes through the global logger rather than one built here, so that
		// whatever the application configures — a level, a handler, common fields
		// such as a service name — reaches this record too. A startup failure is
		// precisely the record that must not fall outside the log setup.
		//
		// There is no chicken-and-egg problem in depending on it: loggear lazy-
		// inits the global logger to a default JSON logger on stdout when nothing
		// has configured one, which is exactly the state a failed startup is in.
		loggear.Error("scheduler startup failed", "error", err)
		os.Exit(1)
	}
}
