package restkit

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/ensoria/loggear/pkg/loggear"
)

// strictDeclarations は「宣言と実挙動の不一致」を即座に失敗させるかどうか。
//
// ドキュメントは Endpoint の宣言から生成されるため、Handle が宣言していない
// ステータスを返すと、生成物が黙って実装から乖離する。宣言を守っているかを
// 実行時に確かめることで、**宣言を動作に関係させる**(= 書き忘れが検出できる)。
//
// 開発環境では即座に失敗させて宣言漏れをテストで見つけ、本番ではリクエストを
// 落とさずログに残す。既定は false(安全側)で、bootstrap が環境に応じて設定する。
var strictDeclarations atomic.Bool

// SetStrictDeclarations は厳格モードを切り替える。アプリ起動時に一度だけ呼ぶ。
func SetStrictDeclarations(strict bool) { strictDeclarations.Store(strict) }

// StrictDeclarations は現在の厳格モードを返す。
func StrictDeclarations() bool { return strictDeclarations.Load() }

// 厳格モードにする環境。本番相当(staging / production)では、宣言漏れが
// リクエストを落とさないようにする。
const (
	envLocal       = "local"
	envTest        = "test"
	envDevelopment = "development"
)

// StrictForEnv は環境名から厳格モードにすべきかを返す。
// 開発者が触っている環境(local / test / development)でのみ厳格にする。
func StrictForEnv(env string) bool {
	switch env {
	case envLocal, envTest, envDevelopment:
		return true
	default:
		return false
	}
}

// checkDeclaredStatus は返そうとしている成功ステータスが宣言済みかを確かめる。
// 宣言漏れは実装のバグなので、厳格モードでは panic、そうでなければエラーログを出す。
func checkDeclaredStatus(status int, success int, responses []ResponseSpec) {
	if isDeclaredStatus(status, success, responses) {
		return
	}
	msg := fmt.Sprintf(
		"undeclared success status %d: declare it in Endpoint.Success or Endpoint.Responses "+
			"so the generated documentation matches the implementation", status)
	if strictDeclarations.Load() {
		panic(msg)
	}
	loggear.Error(msg, "status", status)
}

// isDeclaredStatus は status が Success か Responses のいずれかで宣言されているかを返す。
func isDeclaredStatus(status int, success int, responses []ResponseSpec) bool {
	if success == 0 {
		// Success 未宣言のときの既定(rest.Result.ToResponse と揃える)。
		success = http.StatusOK
	}
	if status == success {
		return true
	}
	for _, r := range responses {
		if r.Status == status {
			return true
		}
	}
	return false
}
