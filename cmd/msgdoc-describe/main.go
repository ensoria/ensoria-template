//go:build msgdoc_describe

// Command msgdoc-describe は describe モードでメッセージング仕様
// (msgdoc.MessagingSpec)を JSON で stdout に出力する。encli が
// `go run -tags msgdoc_describe` で実行して回収し、AsyncAPI へレンダリングする。
//
// apidoc-describe と対になる別コマンドにしてある(判断ポイント D1)。JSON 契約が
// 互いに独立するので、生成が壊れたときにどちらの経路の問題か切り分けやすい。
//
// 本番ビルドには含めない(build tag `msgdoc_describe` で隔離)。
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

	spec, err := describe.BuildMessaging(envVal)
	if err != nil {
		fmt.Fprintln(os.Stderr, "msgdoc-describe: "+err.Error())
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		fmt.Fprintln(os.Stderr, "msgdoc-describe: encode: "+err.Error())
		os.Exit(1)
	}
}
