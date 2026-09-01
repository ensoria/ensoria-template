package restkit

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
)

// strictDeclarations decides whether a mismatch between what an endpoint
// declares and what it answers with fails immediately.
//
// The documentation is generated from the Endpoint declaration, so a Handle
// that answers with a status nobody declared makes the generated document drift
// from the implementation without a word. Checking the declaration while the
// request is served is what makes the declaration load-bearing: forgetting one
// becomes a defect that fails somewhere, instead of a silent drift.
//
// Two things decide its value, and the last one to speak wins:
//
//   - init, below, makes it true inside a test binary and false everywhere else
//   - bootstrap.ApplyGlobalSettings sets it from ENV when an application starts
//
// So a process that serves real requests is governed by its environment, and a
// test — which starts no process and therefore never reaches bootstrap — is
// strict by default.
var strictDeclarations atomic.Bool

// initialStrictDeclarations is the default init gives the flag.
//
// It is kept in a variable of its own because the flag itself cannot stand in
// for the default once the specs that exercise both modes have written to it.
// export_test.go exposes this to the suite, so that the default can be asserted
// on directly.
var initialStrictDeclarations = testing.Testing()

// init makes strict mode the default inside a test binary.
//
// The intent behind strict mode has always been "be strict wherever a developer
// is working", and ENV is only a proxy for that intent — one that missed the
// place that matters most. bootstrap.ApplyGlobalSettings is reached only by
// server.Run and scheduler.Start, while endpoint tests call Controller.Handle
// directly. They therefore ran with the flag at its zero value, and an
// undeclared status went uncaught in the very place the README promises it is
// caught. Asking every suite to switch the mode on would only move the same
// problem: a suite that forgot would be back where it started.
//
// testing.Testing (Go 1.21+) reports whether the program is a test binary, so
// this keeps the production default false — an application that never calls
// bootstrap cannot begin failing requests over a missing declaration — while a
// test binary is strict without anyone asking for it.
//
// That is also why this production package imports "testing": telling a library
// that it is running under test is precisely what testing.Testing was added
// for, and it is what lets the safe production default and the automatic test
// default hold at the same time. The import registers no flags, cmd/server
// already links the package transitively, and the measured effect on the server
// binary was -0.03%. Please do not remove the import as unwanted weight.
func init() { strictDeclarations.Store(initialStrictDeclarations) }

// SetStrictDeclarations switches strict mode. Applications call it once at
// startup, through bootstrap.ApplyGlobalSettings, and it overrides the default
// above: a test that boots an application with ENV=production deliberately gets
// production behaviour, because it asked for it.
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
