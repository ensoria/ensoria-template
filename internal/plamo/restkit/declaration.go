package restkit

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
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

// LogTypeDeclarationDrift labels the record written when an endpoint answers
// with a status it never declared. The template's other records carry a stable
// "type" as well ("access_log", "panic_log"), so that a log platform can be
// given a search and an alert condition that survives a change of wording.
//
// Outside strict mode this record is the only sign that the generated
// documentation has drifted from the implementation, so a production-like
// environment should alert on it. See the README for what to do when it fires.
const LogTypeDeclarationDrift = "declaration_drift_log"

// checkDeclaredStatus は返そうとしている成功ステータスが宣言済みかを確かめる。
// 宣言漏れは実装のバグなので、厳格モードでは panic、そうでなければエラーログを出す。
//
// The request is taken so that the record outside strict mode names the
// endpoint it came from. Without it the record says only that something,
// somewhere, has drifted — which is not enough to act on in production, where
// this record is all anyone gets.
func checkDeclaredStatus(r *rest.Request, status int, success int, responses []ResponseSpec) {
	if isDeclaredStatus(status, success, responses) {
		return
	}
	msg := fmt.Sprintf(
		"undeclared success status %d: declare it in Endpoint.Success or Endpoint.Responses "+
			"so the generated documentation matches the implementation", status)
	if strictDeclarations.Load() {
		// The panic is recovered by the pipeline's recovery middleware, whose
		// record already carries the method and path — so the message alone is
		// enough here.
		panic(msg)
	}
	loggear.Error(msg,
		slog.String("method", r.Method()),
		slog.String("path", r.Path()),
		slog.Int("status", status),
		slog.String("type", LogTypeDeclarationDrift),
	)
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
