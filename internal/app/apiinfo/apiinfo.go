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
	// HTTPDescription は HTTP API の概要(OpenAPI info.description)。空でもよい。
	HTTPDescription = "HTTP API of the Ensoria application."
	// MessagingDescription はメッセージング面の概要(AsyncAPI info.description)。
	// 説明だけを面ごとに分けているのは、タイトル・バージョン・ライセンスが
	// 同じ製品・同じ契約バージョンを指すのに対し、概要文はどの面を説明している
	// のかを述べるものだからである。
	MessagingDescription = "Message broker and WebSocket surface of the Ensoria application."
	// LicenseName は API のライセンス名(OpenAPI info.license.name)。
	// ライセンスを出す場合、OpenAPI では name が必須。既定は非公開 API 向けの
	// プレースホルダなので、公開 API では実際のライセンスに書き換えること。
	LicenseName = "Proprietary"
	// LicenseIdentifier は SPDX ライセンス式(例 "Apache-2.0")。独自ライセンスなら空。
	// OpenAPI 3.1 では LicenseURL と排他で、両方あれば識別子が優先される。
	LicenseIdentifier = ""
	// LicenseURL はライセンス文書の URL。SPDX 識別子が無い場合に使う。
	LicenseURL = ""
)

// Info は HTTP API の宣言されたメタ情報を中立モデルにして返す。
func Info() *apidoc.Info {
	return info(HTTPDescription)
}

// MessagingInfo はメッセージング面のメタ情報を返す。
// Info との違いは概要文だけで、タイトル・バージョン・ライセンスは共有する。
func MessagingInfo() *apidoc.Info {
	return info(MessagingDescription)
}

func info(description string) *apidoc.Info {
	return &apidoc.Info{
		Title:       Title,
		Version:     Version,
		Description: description,
		License: &apidoc.License{
			Name:       LicenseName,
			Identifier: LicenseIdentifier,
			URL:        LicenseURL,
		},
	}
}
