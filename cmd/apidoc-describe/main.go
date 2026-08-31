//go:build apidoc_describe

// Command apidoc-describe は describe モードで API スペック(apidoc.APISpec)を
// JSON で stdout に出力する。encli が `go run -tags apidoc_describe` で実行して回収し、
// docai / OpenAPI などの各フォーマットへレンダリングする。
//
// 本番ビルドには含めない(build tag `apidoc_describe` で隔離)。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ensoria/ensoria-template/internal/app/bootstrap/describe"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/spf13/pflag"
)

func main() {
	// 既定 local はディスクの internal/config/.env(dotenv)から設定を読むため、
	// OS 環境変数(direnv)無しで describe を実行できる。encli は --env で上書き可。
	envVal := pflag.StringP("env", "e", "local", "config environment: [local], [development], [staging], [production] or [test].")
	pflag.Parse()

	// This command's stdout is a JSON contract, so nothing else may be written
	// there. Building the specification resolves the constructors the project
	// wrote, and one loggear call in any of them would land in the middle of the
	// document — which reaches encli as a parse error, a long way from its cause.
	//
	// The destination is fixed here rather than left to loggear's default, so
	// that the constraint sits where the contract is and this command does not
	// depend on a default it does not own.
	loggear.ConfigureSlog(loggear.WithOutput(os.Stderr))

	spec, err := describe.BuildHTTP(envVal)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apidoc-describe: "+err.Error())
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		fmt.Fprintln(os.Stderr, "apidoc-describe: encode: "+err.Error())
		os.Exit(1)
	}
}
