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
	"github.com/spf13/pflag"
)

func main() {
	// 既定 local はディスクの internal/config/.env(dotenv)から設定を読むため、
	// OS 環境変数(direnv)無しで describe を実行できる。encli は --env で上書き可。
	envVal := pflag.StringP("env", "e", "local", "config environment: [local], [development], [staging], [production] or [test].")
	pflag.Parse()

	spec, err := describe.Build(envVal)
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
