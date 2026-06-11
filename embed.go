package ensoriatemplate

import (
	"embed"
	"io/fs"
	"os"

	"github.com/ensoria/config/pkg/env"
)

// embeddedConfigFS には、Spec YAML（各環境の `${env}.yml`）のみを埋め込む。
// `.env` は go:embed がドットファイルを自動除外するため埋め込まれない（値は環境変数や
// SecretsManager / Parameter Store から取得し、イメージには焼かない）。
// go:embed は `..` を使えず埋め込み元ファイルと同階層以下しか対象にできないため、
// `internal` を子に持つモジュールルートにこのファイルを置いている。
//
//go:embed internal/config internal/module/*/config internal/query/*/config
var embeddedConfigFS embed.FS

// ConfigFS は設定の読み込み元となる fs.FS を返す。
// local / test ではディスクからライブ読み込み（os.DirFS）し、`.env` の変更や
// YAML の編集を再ビルドなしで反映できるようにする。それ以外（development / staging /
// production）では、自己完結したイメージにするため埋め込み済みの Spec YAML を使う。
func ConfigFS(envVal string) fs.FS {
	switch env.Environment(envVal) {
	case env.Local, env.Test:
		return os.DirFS(".")
	default:
		return embeddedConfigFS
	}
}
