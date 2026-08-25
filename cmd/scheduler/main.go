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
		// standard library's unstructured stderr line. The logger is built here
		// with no options — JSON on stdout — because the configuration that
		// would otherwise shape it is the very thing that may have failed.
		loggear.NewSlogLogger().Error("scheduler startup failed", "error", err)
		os.Exit(1)
	}
}
