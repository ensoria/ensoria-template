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
| **任意（動作に影響）** | 省略は「そうしない」という選択 | `Success` / `Produces` / `BodyRules` / `PathRules` / `QueryRules` / `Security` |
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

### 誰が呼べるか: `Security`

**宣言しないエンドポイントは「要認証」になります。** 検証済みの呼び出し元が無ければ
アダプタが 401 を返します。認証について何も考えなかったエンドポイントは、開くのではなく
閉じる側に倒れます。

そのため、**公開エンドポイントは公開だと明示的に書く必要があります**。

```go
// 公開: 誰でも呼べる
Security: &restkit.SecuritySpec{Public: true},

// 要認証 + スコープ: 宣言したスコープを「すべて」持つ呼び出し元だけ通す
Security: &restkit.SecuritySpec{Scopes: []string{"users:write"}},

// 資格情報の種類も限定する場合
Security: &restkit.SecuritySpec{
	Schemes: []string{authkit.SchemeJWT},
	Scopes:  []string{"users:write"},
},

// 宣言なし = 要認証(スコープの指定は無し)
```

判定はこうなります。

| 状況 | 応答 |
|---|---|
| `Public: true` | 呼び出し元の有無に関わらず処理する |
| 呼び出し元が未認証 | **401** `unauthenticated` |
| 呼び出し元は認証済みだがスキームが不一致 | **403** `forbidden` |
| 呼び出し元は認証済みだがスコープ不足 | **403** `forbidden` |

401 と 403 は別の意味です。401 は「あなたが誰か名乗ってください」、403 は「あなたが誰かは
分かった上で、それはできません」です。403 のときに資格情報を付け直しても結果は変わりません。

この判定は**検証よりも先**に走ります。未認証の呼び出し元に、どんなフィールドがあり
どう制約されているかを教えないためです。

`Scopes` は文書化のためだけの宣言ではなく、実際に強制されます。書けば動作が変わるので、
書き忘れても気づかない、ということが起きません。

> **認証の設定が要ります。** 要認証のエンドポイントが1つでもあるのに認証が未設定
> （`AUTH_MODE` / `AUTH_SECRET` / `AUTH_JWKS_URL` / `AUTH_API_KEYS` のいずれも無い）だと、
> アプリケーションは起動時に停止します。全リクエストを拒否し続ける設定ミスを、
> 最初のリクエストではなく起動時に潰すためです。ローカル開発用の既定値は
> `internal/config/.env` に入っています。

> **生 Controller には効きません。** `Security` は `restkit.Endpoint` のアダプタが評価します。
> `rest.Controller` を自分で実装した場合はこの判定を通らないので、認可は自分で書く必要が
> あります。テンプレート内のコントローラはすべて型付きエンドポイントです。

### 検証は宣言するだけ

`BodyRules` / `PathRules` / `QueryRules` に宣言した検証は、`Handle` が呼ばれる**前**に実行されます。
違反した場合はアダプタが 422 とフィールド単位のエラーを返すので、`Handle` の中で
ステータスやメッセージを組み立てる必要はありません。

各フィールドの詳細（必須・任意・条件付き必須の別を含む）は
[endpoint.go](internal/plamo/restkit/endpoint.go) の doc コメントに書いてあります。

### リクエストのフィールドを必須にする

JSON では**キーが無いことと、ゼロ値が入っていることを区別できません**。`{"count": 0}` と
`{}` は、どちらも Go の `int` フィールドでは `0` になります。そのため必須の宣言方法は
フィールドの型によって変わります。

| フィールドの型 | 必須の宣言 | 判定 |
|---|---|---|
| `string` | `vkit.Required()` | 空文字列を「未指定」とみなす |
| `[]T` | `vkit.SliceNotEmpty()` | 空スライスを「未指定」とみなす |
| **ポインタ（型は問わない）** | `vkit.NotNil()` | **キーの有無だけを見る。ゼロ値でも通る** |
| 非ポインタの数値 | `vkit.NumNotZero()` | 0 を「未指定」とみなす（後述の制限あり） |

「0 や空文字列を正当な値として受け取りたいが、指定は必須」という場合は、
**フィールドをポインタにして `NotNil()`** を使ってください。これが唯一、
「指定しなかった」と「ゼロ値を指定した」を区別できる形です。

```go
// dto
type PaymentCallback struct {
	OrderId *int `json:"orderId"`
}

// エンドポイント
BodyRules: []*rule.RuleSet{
	{Field: "orderId", Rules: []rule.Rule{vkit.NotNil(), vkit.MinValue(1)}},
},

// Handle の中
id := *req.OrderId
```

> ⚠️ **`NotNil()` は非ポインタのフィールドでは何も強制しません。** 非ポインタには常に値が
> あるため、必ず成功します。ドキュメントには「必須」と出るのに実際には素通りする、という
> 食い違いが起きるので、`NotNil()` を書いたらフィールドがポインタか確認してください。

> **`NumNotZero()` は妥協手段です。** 0 を「未指定」と同義に扱うので、0 が正当な値である
> フィールド（件数・残高・座標など）には使えません。既存の DTO をポインタに変えたくない
> 場合の逃げ道として用意しています。

**制約と必須は別の宣言です。** `MinValue(1)` のような制約は「値があるとき、それがどうで
あるべきか」だけを述べます。値が無ければ検証されずに通るので、「任意だが、指定されたら
1 以上」がそのまま書けます。必須にしたい場合は上記の必須ルールを併記してください。

### 部分更新（PATCH）: `optional.Optional[T]`

部分更新では、**「そのフィールドに触れない」と「そのフィールドを空にする」を区別する**
必要があります。JSON では `{}` と `{"nickname": null}` の差ですが、ポインタでは
どちらも `nil` になって表せません。

`optional.Optional[T]` はこの3状態を保持します。

```go
type UpdateUser struct {
	Name     optional.Optional[string] `json:"name"`
	Nickname optional.Optional[string] `json:"nickname"`
}
```

| リクエスト | `IsSet()` | `Get()` | 意味 |
|---|---|---|---|
| `{}` | `false` | `_, false` | 触れない |
| `{"nickname": null}` | `true` | `_, false` | 空にする |
| `{"nickname": "taro"}` | `true` | `"taro", true` | その値にする |

`Handle` では **`IsSet()` を見ないと、送られなかったフィールドをゼロ値で上書きします**。

```go
if name, ok := req.Name.Get(); ok {
	user.Name = name          // 値が送られてきた場合だけ反映する
}
if req.Nickname.IsSet() {
	// 送られてきた。値があれば設定、null なら削除
}
```

フィールドごとに許す操作は宣言で変えられます。

```go
BodyRules: []*rule.RuleSet{
	// 省略はできるが、送るなら値が要る（null で消すことは許さない）
	{Field: "name", Rules: []rule.Rule{vkit.NotNullIfSet(), vkit.MaxLength(10)}},
	// nickname は宣言なし = 省略も null も値も許す
},
```

生成ドキュメントには、`Required` 列（省略できるか）と `Nullable` 列（null にできるか）
の組み合わせとして出ます。OpenAPI では `type: ["string", "null"]` と `required` の
有無で同じことを表現します。

> **`Optional[T]` は部分更新のための型です。** 通常の作成・更新（POST / PUT）では
> 「未指定」と「null」を区別する理由が無いので、ポインタ + `NotNil()` で十分です。
> 区別が要らない場所で使うと、`Handle` の分岐が無駄に増えます。


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



