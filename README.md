# Ensoriaフレームワーク プロジェクトテンプレート

`encli install [directory_name]`でインストールされるプロジェクトのテンプレートです。

現在実装中。

テストするためにpublicにしてあります。

フレームワークが出来上がるのを楽しみにしてくださいね！


## HTTP エンドポイントの書き方（`restkit.Endpoint`）

HTTP エンドポイントは [`restkit.Endpoint[Req, Res]`](internal/plamo/restkit/endpoint.go) として宣言します。
`Req` がリクエストボディの型、`Res` が成功時レスポンスボディの型です。ボディを持たないエンドポイントは
どちらにも `restkit.NoBody` を使います。

### 最小構成

**必須なのは `Handle` だけ**です。次のエンドポイントはこれだけで動きます。

```go
func NewGet(svc service.UserService) *restkit.Endpoint[restkit.NoBody, dto.GetUser] {
	return &restkit.Endpoint[restkit.NoBody, dto.GetUser]{
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[dto.GetUser], error) {
			return rest.NewResult(&dto.GetUser{ID: 1, Name: "hoge"}), nil
		},
	}
}
```

### フィールドの分類

残りのフィールドは3種類に分かれます。**ドキュメントを生成しないプロジェクトは、
「ドキュメント専用」の欄をすべて省略できます**（動作は一切変わりません）。

| 分類 | 意味 | フィールド |
|---|---|---|
| **必須** | 無いと動かない | `Handle` |
| **任意（動作に影響）** | 省略は「そうしない」という選択 | `Success` / `Produces` / `BodyRules` / `PathRules` / `QueryRules` |
| **任意（ドキュメント専用）** | 生成器だけが読む。省略しても動作は同じ | `Summary` / `Description` / `FieldDocs` / `Task` / `AlsoRead` / `Related` / `IDPrefix` / `ResponseHeaders` / `Errors` / `Behavior` |
| **条件付き必須** | 下記参照 | `Responses` |

とくに紛らわしい2つに注意してください。

- **`ResponseHeaders` はヘッダを送りません。** ドキュメントに載せるだけです。
  実際に送るのは `Handle` の中の `rest.WithHeader(...)` です。
- **`Errors` はステータスを決めません。** 実際のステータスは `Handle` が返すエラー
  （`restkit.HTTPError` を実装したもの）で決まります。`Errors` はそれを文書化するだけです。

### 条件付き必須: `Responses`

`Handle` が `Success` **以外**のステータスを返し得る場合、そのステータスは `Responses` に宣言が必要です。

```go
Success: http.StatusCreated,
Responses: []restkit.ResponseSpec{
	{Status: http.StatusAccepted, When: "The user was queued for asynchronous creation"},
},
// Handle の中:
return rest.NewResult(&user, rest.WithStatus(http.StatusAccepted)), nil
```

宣言していないステータスを返すと、**local / test / development ではアダプタが即座に失敗させます**
（staging / production ではエラーログを出しつつ、そのステータスを返します）。

これは「ドキュメント専用の宣言は書き忘れる」という問題への対策です。書き忘れが静かな
ドキュメント乖離ではなく、テストで落ちる欠陥になります。

### 検証は宣言するだけ

`BodyRules` / `PathRules` / `QueryRules` に宣言した検証は、`Handle` が呼ばれる**前**に実行されます。
違反した場合はアダプタが 422 とフィールド単位のエラーを返すので、`Handle` の中で
ステータスやメッセージを組み立てる必要はありません。

各フィールドの詳細（必須・任意・条件付き必須の別を含む）は
[endpoint.go](internal/plamo/restkit/endpoint.go) の doc コメントに書いてあります。


## API ドキュメントの生成

HTTP API のドキュメントは、**実装から自動生成**します。アノテーションやコメントを書く必要はありません。

```sh
encli generate docai      # LLM 向けドキュメント一式（docs/INDEX.md ほか）
encli generate openapi    # OpenAPI 3.1（docs/openapi.yaml）
```

### 仕組み

どちらのコマンドも、`cmd/apidoc-describe` を `go run -tags apidoc_describe` で実行し、
**リフレクションで API 仕様（型・検証ルール・ルーティング宣言）を書き出した中立モデル**を回収してから、
それぞれの形式にレンダリングします。

describe は build tag `apidoc_describe` で本番ビルドから隔離されており、
**DB やメッセージブローカーには接続しません**（接続系はスタブが注入され、fx のライフサイクルも起動しません）。
そのためインフラを立ち上げずにドキュメントを生成できます。

### 生成物の元になる宣言

型から導けない情報は、コードの宣言が唯一の出所です。ドキュメントに `TODO` が出たら、
対応する宣言が未記入だという意味です。

| 出力される内容 | 宣言する場所 |
|---|---|
| API のタイトル・バージョン・概要・ライセンス | [internal/app/apiinfo](internal/app/apiinfo/apiinfo.go) |
| 概要・説明・フィールドの意味・関連エンドポイント | `restkit.Endpoint` の `Summary` / `Description` / `FieldDocs` / `Related` |
| 副作用・冪等性・前提条件・認可スコープ | `restkit.Endpoint` の `Behavior` |
| エンドポイント固有のエラー | `restkit.Endpoint` の `Errors` |
| リクエスト/レスポンスの型・必須・制約 | 型パラメータ `Endpoint[Req, Res]` と `BodyRules` / `PathRules` / `QueryRules` |

`internal/app/apiinfo/apiinfo.go` はプロジェクトごとに書き換える前提のファイルです。
既定値はプレースホルダなので、**API の名前・バージョン・ライセンスは最初に設定してください**。

### 上書きされないファイル

生成物には目印（docai はメタスタンプ行、OpenAPI は `x-generated`）が入ります。
目印の無い既存ファイルは**手書きとみなして上書きしません**ので、`docs/` 配下に手書きの補足を置けます。


## サーバタイムアウト

HTTPサーバのタイムアウトは **2層** で構成されています。値はすべて config（環境変数）から設定でき、duration 文字列（例: `"30s"`, `"2m"`）で指定します。

### Layer 1: コネクションレベル（`http.Server`）

[internal/app/http/http.go](internal/app/http/http.go) の `NewHTTPApp` で `http.Server` に設定されます。

| 環境変数 | フィールド | 既定値 | 説明 |
|---|---|---|---|
| `HTTP_READ_HEADER_TIMEOUT` | `ReadHeaderTimeout` | `10s` | リクエストヘッダ読み込みの上限（Slowloris 対策） |
| `HTTP_READ_TIMEOUT` | `ReadTimeout` | `30s` | リクエスト全体（ボディ含む）の読み込み上限 |
| `HTTP_WRITE_TIMEOUT` | `WriteTimeout` | `0`（無効） | レスポンス書き込み全体の絶対 deadline |
| `HTTP_IDLE_TIMEOUT` | `IdleTimeout` | `120s` | keep-alive のアイドル上限 |

> ⚠️ **`WriteTimeout` は既定で 0（無効）です。** これはレスポンス書き込み全体の絶対 deadline であり、SSE・WebSocket・大きなファイルダウンロードのような長時間接続を切断してしまうためです。リクエスト単位のタイムアウトは Layer 2 で制御します。

### Layer 2: リクエスト単位（pipeline）

| 環境変数 | フィールド | 既定値 | 説明 |
|---|---|---|---|
| `HTTP_HANDLER_TIMEOUT` | `HandlerTimeout` | `30s` | コントローラ/ミドルウェアチェーンの実行（=レスポンスの計算）の上限。0 で無効 |

超過するとクライアントへ `503 Service Unavailable` を返します（[CreateHTTPPipeline](internal/app/http/http.go) で `pipeline.HTTP.Timeout` / `TimeoutResponse` として注入）。

- **ストリーミング・WebSocket は対象外**です。ストリーミング/ファイルレスポンスは「計算」の後に書き込まれるため上限の対象外、WebSocket は別ルータ（`wsrouter`）のため影響を受けません。
- **重要**: タイムアウトでクライアントにはレスポンスが返りますが、打ち切られたコントローラの処理自体を中断させるには、コントローラが `r.Context()` を下流（DB クエリ・外部 HTTP 呼び出し等）へ伝播させる必要があります。詳細は `rest` の README「Request Timeout」を参照してください。

## .envファイルの注意事項

`.env`ファイルは、ローカル環境、テスト環境でのみ利用することが想定されています。
それ以外の環境が、`.env`の値を使うことを想定せずに実装してください。

特に、`encli build migration`で出力する設定ファイルには、`.env`は含まれません。



