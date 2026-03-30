package module

import (
	_ "github.com/ensoria/ensoria-template/internal/module/order"
	_ "github.com/ensoria/ensoria-template/internal/module/post"
	_ "github.com/ensoria/ensoria-template/internal/module/user"
)

func init() {
	// モジュール全体の初期化のタイミングで実行したい処理を実装
}
