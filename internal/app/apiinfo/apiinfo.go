// Package apiinfo は API 全体のメタ情報(タイトル・バージョン・概要)を宣言する。
//
// 型やルーティングからは導けないため、ここでの宣言が唯一の出所になる。describe が
// APISpec に注入し、OpenAPI の `info` などドキュメント生成で使われる。
// プロジェクトに合わせて書き換えること。
package apiinfo

import "github.com/ensoria/ensoria-template/internal/plamo/apidoc"

const (
	// Title は API の名前(OpenAPI info.title)。
	Title = "Ensoria API"
	// Version は API 契約のバージョン(OpenAPI info.version)。
	// アプリケーション実装のバージョンとは別物で、API の互換性を表す。
	Version = "0.1.0"
	// Description は API の概要(OpenAPI info.description)。空でもよい。
	Description = "HTTP API of the Ensoria application."
)

// Info は宣言されたメタ情報を中立モデルにして返す。
func Info() *apidoc.Info {
	return &apidoc.Info{
		Title:       Title,
		Version:     Version,
		Description: Description,
	}
}
