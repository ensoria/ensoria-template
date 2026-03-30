package main

import (
	"log"

	"github.com/ensoria/ensoria-template/internal/app/bootstrap/scheduler"
	"github.com/spf13/pflag"
)

func main() {
	// FIXME: configのenvを使って、ここのリストを修正する
	// envList := slices.Join(env.StringList, ", ")
	envVal := pflag.StringP("env", "e", "local", "it must be either [local], [develop], [staging], [production] or [testing].")
	pflag.Parse()

	if err := scheduler.Start(envVal); err != nil {
		log.Fatal(err)
	}
}
