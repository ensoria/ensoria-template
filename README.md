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

宣言していないステータスを返すと、**`go test` のバイナリでは `ENV` に関係なく必ず失敗します**
（アプリのプロセスでは local / test / development で失敗し、staging / production では
エラーログを出しつつそのステータスを返します）。

これは「ドキュメント専用の宣言は書き忘れる」という問題への対策です。書き忘れが静かな
ドキュメント乖離ではなく、テストで落ちる欠陥になります。

#### Where the check fails, and why the tests come first

The check has two storeys:

| Where | When an undeclared status fails |
|---|---|
| A `go test` binary | **Always**, whatever `ENV` says |
| A server or scheduler process | When `ENV` is `local`, `test` or `development` |

The first storey is what makes the promise above hold. `restkit` reads
`testing.Testing()` when it initialises, so **a test binary checks declarations
by default**: an endpoint suite inherits the check without switching anything
on, including one that calls `ctrl.Handle(r)` directly and starts no process,
and including a suite written long after this was decided. A forgotten
declaration therefore fails in the place a defect is supposed to surface, with a
message that names the missing status:

```
[PANICKED] undeclared success status 202: declare it in Endpoint.Success or
Endpoint.Responses so the generated documentation matches the implementation
```

That message matters as much as the failure. Without the check, a test that
asserts on the status fails with `Expected 202 to equal 200` — which reads as
"the handler returns the wrong status" and sends you to fix the handler, while
the actual defect is the missing declaration. A test that does not assert on the
status does not fail at all.

The second storey is the same check reaching a developer running the server by
hand, and there `ENV` governs it: a production process must keep serving the
request and leave a record, rather than fail it over a documentation defect. The
process setting is applied at startup, after the default, so a test that
deliberately boots an application with `ENV=production` gets production
behaviour — it asked for it.

#### Alert on the drift record in production

In staging and production nothing fails, so that one error record is the only
sign the generated documentation no longer matches the implementation. **Make it
an alert.** It carries a stable field to match on, next to the method, path and
status it drifted with:

```json
{"level":"ERROR","msg":"undeclared success status 201: ...","method":"POST",
 "path":"/order","status":201,"type":"declaration_drift_log"}
```

Alert on `type = "declaration_drift_log"` — matching the message text instead
would break the next time the wording changes. The same field naming is used by
the access log (`access_log`) and the panic log (`panic_log`).

To fix one when it fires: take the `method` and `path` from the record, find that
endpoint, and declare the status the record names — as `Success` if it is the
endpoint's ordinary answer, or as an entry in `Responses` if it is one of
several. Then regenerate the documentation. Nothing else has to change: the
handler was already returning that status, and callers were already receiving
it. What was missing was the declaration the documentation is generated from.

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

> **認証の設定が要ります。** 要認証のエンドポイントが1つでもあるのに**呼び出し元を検証
> できるものが何も無い**と、アプリケーションは起動時に停止します。全リクエストを拒否し
> 続ける設定ミスを、最初のリクエストではなく起動時に潰すためです。
> 同様に、`Schemes` で限定した資格情報を検証できない場合も停止します。
> ローカル開発用の既定値は `internal/config/.env` に入っています。

> **生 Controller には効きません。** `Security` は `restkit.Endpoint` のアダプタが評価します。
> `rest.Controller` を自分で実装した場合はこの判定を通らないので、認可は自分で書く必要が
> あります。テンプレート内のコントローラはすべて型付きエンドポイントです。

### 認証の設定

呼び出し元の検証は2種類あり、片方だけでも両方でも使えます。設定値そのものの説明は
[config の README](https://github.com/ensoria/config#認証の設定auth_) にあります。

| 種類 | 想定する呼び出し元 | 何が身元になるか |
|---|---|---|
| **JWT** | 人間の利用者（ブラウザ・モバイル） | トークンの `sub` / `scope` クレーム |
| **API キー** | 他のサーバ（サービス間通信） | キーそのもの |

**`AUTH_SECRET` は利用者ごとの鍵ではありません。** JWT の署名鍵で、アプリケーションに1つです。
利用者が1万人いても鍵は1つで、誰なのかはトークンの中身に入っています。また `hs256` は共有鍵
（鍵を持てばトークンを偽造できる）なので**ローカル開発向け**です。本番は `jwks` を使い、
IdP の公開鍵で検証してください。

> **このテンプレートはトークンを発行しません。** 発行（ログイン）は IdP か、自分で書く
> ログインエンドポイントの仕事です。`Auth` に署名鍵や有効期限の設定が無いのはそのためで、
> **アプリケーション自身にとって `AUTH_SECRET` は検証専用**です。
>
> 開発時にトークンが要るときは `encli auth token` を使ってください。これはアプリケーションの
> 外側にある開発用ツールで、`AUTH_SECRET` で署名した短命のトークンを発行します。
>
> ```sh
> TOKEN=$(encli auth token --sub alice --scope users:read)
> curl -H "Authorization: Bearer $TOKEN" localhost:8080/users/1
> ```
>
> `local` と `test` 環境、かつ `AUTH_MODE=hs256` のときにしか動きません（上書きするフラグは
> ありません）。`jwks` ではアプリケーションは発行元の公開鍵しか持たず署名できないため、
> トークンは発行元から取得します。

テンプレート同梱の `AUTH_SECRET` / `AUTH_API_KEYS` は公開値なので、`local` と `test` 以外の
環境ではそのままだと起動を止めます。ただし**判定するのは実際に使われる値だけ**です。
`AUTH_MODE=jwks` では `AUTH_SECRET` はどこからも読まれないため、値が残っていても止まりません
（`hs256` に戻した時点で再び止まります）。`AUTH_API_KEYS` は `AUTH_MODE` に関係なく読まれるので、
どのモードでも同梱キーは拒否されます。

#### API キーの保管を差し替える（`KeyStore`）

`AUTH_API_KEYS` に並べたキーは、**呼び出し元を識別しません**。既定の実装は「通す / 通さない」
だけを返すので、3社にキーを配ってもログには同じ呼び出し元としか残らず、社ごとに権限を
変えることもできません。

実運用でキーを配るなら [`authkit.KeyStore`](internal/plamo/authkit/verifier.go) を実装して
差し替えます。インターフェースは1メソッドだけです。

```go
type KeyStore interface {
	Lookup(key string) (*Principal, error)
}
```

```go
func (s *dbKeyStore) Lookup(key string) (*authkit.Principal, error) {
	client, err := s.repo.FindByKeyHash(hash(key))
	if err != nil {
		return nil, err
	}
	return &authkit.Principal{
		Subject: client.ID,      // ログに残る「どの取引先か」
		Scheme:  authkit.SchemeAPIKey,
		Scopes:  client.Scopes,  // Endpoint.Security のスコープ判定がそのまま効く
	}, nil
}
```

> **⚠ `Lookup` は毎回、呼び出し元ごとに新しい `Principal` を返してください。**
> 手元のキャッシュや表に入っている `*Principal` を**そのまま返さない**ことが重要です。
>
> 返した `Principal` は `authkit.WithPrincipal` でリクエスト context に載り、
> アプリケーションコードが `PrincipalFrom` で取り出して自由に触れます。
> **そこで `Scopes` に `append` されると、同じキーを使う次のリクエストの権限が広がります。**
> `Scopes` はスライスなので、共有インスタンスを返していると変更がそのまま残ります。
>
> ```go
> // ✗ 表の *Principal をそのまま返している
> func (s *cachedKeyStore) Lookup(key string) (*authkit.Principal, error) {
> 	return s.principals[key], nil // 呼び出し側の変更が次のリクエストに残る
> }
>
> // ⭕️ 毎回組み立て直す（スライス・マップは clone する）
> func (s *cachedKeyStore) Lookup(key string) (*authkit.Principal, error) {
> 	p, ok := s.principals[key]
> 	if !ok {
> 		return nil, errors.New("unknown API key")
> 	}
> 	return &authkit.Principal{
> 		Subject: p.Subject,
> 		Scopes:  slices.Clone(p.Scopes),
> 		Scheme:  p.Scheme,
> 		Claims:  maps.Clone(p.Claims),
> 	}, nil
> }
> ```
>
> 上の `dbKeyStore` の例のように**毎回 DB から読んで組み立てる実装なら自然に満たせます**。
> 注意が要るのは、キーの表をメモリに持つ実装（キャッシュ、起動時ロード）です。
> [`plamo/stub` の `APIKeyStore`](internal/plamo/stub/apikey.go) はまさにそれなので、
> 構築時と `Lookup` の両方で `Principal` をコピーしています。
>
> なお `authkit` 既定の実装（`AUTH_API_KEYS` を使うもの）も毎回新しい `Principal` を作るため、
> この問題は起きません。

差し替えは [internal/app/auth/auth.go](internal/app/auth/auth.go) の1行です。

```go
// 既定（開発用のスタブ。local / test 以外では nil = 設定のキー）
keys, err := devKeyStore(*envVal, params.Auth)
if err != nil {
	return nil, err
}
return authkit.NewVerifier(params.Auth, keys)

// 差し替え後
return authkit.NewVerifier(params.Auth, apikey.NewDBKeyStore(repo))
```

#### 開発用のスタブ `KeyStore`

上の `KeyStore` は DB を用意しないと書けませんが、**ローカルでは
[`plamo/stub`](internal/plamo/stub/apikey.go) のスタブが既定で入っています**。
固定の表から呼び出し元を返すだけの実装で、DB も外部サービスも要りません。

**`local` と `test` 以外では構築できません**（コンストラクタがエラーを返します）。
キーがソースや設定に書かれている以上、それ以外の環境では何も守れないためです。
「`stub` という名前だから本番では使わないだろう」は期待であって保証ではないので、
仕組みで閉じています。

配られるキーは2つです。

| キー | `sub` | スコープ | 出どころ |
|---|---|---|---|
| `AUTH_API_KEYS` に並べたキー（同梱キーを含む） | `local-dev` | `orders:read` `orders:write` `users:read` `users:write` | 設定。**ローカルでキーを足せばそのまま使えます** |
| `ensoria-local-development-payment-provider-key` | `payment-provider` | `orders:write` **のみ** | スタブの中だけ。設定には**入っていません** |

2つ目は `POST /order/payment-callback` のためにあります。このエンドポイントは
`Schemes: [apiKey]` と `Scopes: [orders:write]` を宣言していますが、**プロジェクトの
API キーが何でもできてしまうと、この宣言の意味が確かめられません**。権限を絞ったキーが
1つあることで、「認証は通るが権限が足りない」を実際に観測できます。

```sh
# 決済事業者のキー → 通る
curl -i -X POST localhost:8080/order/payment-callback \
  -H "X-API-Key: ensoria-local-development-payment-provider-key" \
  -H "Content-Type: application/json" \
  -d '{"paymentId":"pay_1","orderId":1,"status":"settled"}'
# HTTP/1.1 204 No Content

# 同じキーで GET /order → 403（orders:read を持たない）
curl -i localhost:8080/order \
  -H "X-API-Key: ensoria-local-development-payment-provider-key"
# HTTP/1.1 403 Forbidden

# 知らないキー → 401
curl -i -X POST localhost:8080/order/payment-callback \
  -H "X-API-Key: nope" -H "Content-Type: application/json" -d '{}'
# HTTP/1.1 401 Unauthorized
```

> **なぜ設定のキーだけでは 403 になるのか。** `AUTH_API_KEYS` に並べたキーは
> **スコープを持てません**（`authkit` の既定の実装は「通す / 通さない」だけを返します）。
> そのため `Scopes` を宣言したエンドポイントは、認証が通ったあとに 403 を返します。
> スタブはここを埋めるためのもので、**本番で同じことをするのが上の `KeyStore` 実装**です。

スタブが入るのは、次のすべてを満たすときだけです。それ以外では `nil` が渡り、
**このスタブが存在しなかったときとまったく同じ動作**になります。

- `local` または `test` 環境である
- `AUTH_API_KEYS` に1つ以上キーがある（API キーを使わない設定を、ここで勝手に有効化しない）
- `AUTH_API_KEYS_EXTERNAL` が立っていない（自前の `KeyStore` を隠さない）

**設定は次のようにします。**

| 環境変数 | 値 | 理由 |
|---|---|---|
| `AUTH_API_KEYS_EXTERNAL` | `true` | **必須。** 立てないと起動時に停止します（後述） |
| `AUTH_API_KEYS` | **空のまま** | 差し替えた時点で無視されます。値を残すと「このキーで入れる」という誤解を招きます |
| `AUTH_API_KEY_HEADER` | 既定のままで可 | ヘッダ名は差し替え後も設定から読まれます |

`AUTH_API_KEYS_EXTERNAL` が必要なのは、**設定しか読めない処理があるため**です。実行中の
アプリケーションは検証器に「何を検証できるか」を直接聞けますが、ドキュメント生成
（`encli generate ...`）は DB に繋がずに設定だけを読みます。この宣言が無いと、設定にキーが
1つも無いことから「API キーは使えない」と判断され、生成される OpenAPI から
`securitySchemes` の API キーが消えます。

実装するときの注意点です。

- **キーは平文で保存しない。** ハッシュで保存し、照合もハッシュで行います
- **`Lookup` は毎リクエスト呼ばれます。** DB に毎回問い合わせると負荷になるので短時間の
  キャッシュを検討してください。ただしキャッシュ時間がそのまま失効の反映遅延になります
- **エラーの中身は外に漏れません。** アダプタが 401 に丸め、本文は理由を明かしません。
  `Lookup` のエラーに内部情報を書いてもクライアントには出ません（ログには出ます）

#### Cookie authentication (browser sessions)

A browser needs a credential it can hold between requests, and neither of the two
above will do: a JWT put where JavaScript can read it is readable by anything
injected into the page, and an API key is a machine's credential. What a browser
gets instead is an opaque id in an `HttpOnly` cookie, matched against a record on
the server.

Naming a store turns it on. There is no separate enable switch, because two keys
expressing one fact can disagree:

```sh
AUTH_SESSION_STORE=redis   # redis, or memory (local and test only)
```

The browser presents its token **once**, and is served by cookie from then on.
Because the record lives on the server, signing out takes effect on the next
request rather than whenever the token would have expired.

##### The two routes the framework serves

| Route | Accepts | Answers |
|---|---|---|
| `POST /session` | `Schemes: [jwt]` — a bearer token | `201` + `Set-Cookie` |
| `DELETE /session` | `Schemes: [session]` — the cookie | `204` + the instruction to drop it |

**`/session` is where this framework puts the session exchange, and a project
should treat both methods on that path as taken.** They are registered by
[internal/app/auth/api](internal/app/auth/api/module.go), which
[internal/app/bootstrap/server](internal/app/bootstrap/server/server.go) imports
for its `init()`. If you declare a module of your own on `/session`, both are
registered on the same path and the collision is yours to discover at runtime.

> `DELETE /session` answers `401`, not `204`, when there is no live session —
> `Schemes: [session]` means a verified caller is required, and a request with no
> cookie has none. **The response still carries the instruction to drop the
> cookie**, so both answers leave the caller signed out; a client can treat them
> identically. Signing out twice therefore produces one `204` and one `401`.

##### Changing the path

`/session` is **not reserved by machinery**. Nothing in the framework routes,
dispatches or looks up by that literal — it is a declared route like any other,
written once:

```go
// internal/app/auth/api/module.go
const SessionPath = "/session"
```

Change that constant and the endpoints move. The generated documentation follows
on its own: the `session` security scheme's description quotes `SessionPath`
rather than a copy of it, so `securitySchemes` keeps naming the path you actually
serve.

**Prefer not to, though.** Every example in this README, the identity-provider
setup, and anything `encli` adds later assume `/session`, so a project that moves
it diverges from its own framework's documentation for as long as it lives.
Change it when `/session` collides with a resource you own — a `Session` model of
your own, say — and not for taste.

##### Turning cookie authentication off

An application whose callers are all services or mobile clients turns it off in
**two places, and both are required**:

1. unset `AUTH_SESSION_STORE`
2. remove the blank import of `internal/app/auth/api` from
   [internal/app/bootstrap/server](internal/app/bootstrap/server/server.go) —
   **and from [internal/app/bootstrap/describe/doc.go](internal/app/bootstrap/describe/doc.go)**,
   or the generated documents keep describing two endpoints you no longer serve

Neither does it alone, and the combinations are not symmetric:

| `AUTH_SESSION_STORE` | Blank import | Result |
|---|---|---|
| set | kept | Cookie authentication, the normal setup |
| set | removed | Boots. Session cookies are still **verified**; nothing can **create** one. Useful when another service issues them |
| unset | kept | **Refused at startup.** The endpoints would answer `503` to every sign-in |
| unset | removed | Cookie authentication off |

**Leave the two `session.*` constructors in the dependency graph either way.**
They answer `nil` when `AUTH_SESSION_STORE` is unset, which is exactly what "off"
looks like:

```go
session.NewSessionStore(envVal),   // required: NewVerifier takes a sessionkit.Store
session.NewSessionCookies,         // may be removed once the blank import is gone
```

Deleting `NewSessionStore` does not turn anything off — it fails the graph with
`missing type: sessionkit.Store`, because the verifier asks for one whether or
not sessions are configured.

##### What a cookie forces you to configure

A cookie is attached by the browser to **every** request to this origin, whoever
caused it — including a form on another site. That is the one weakness cookies
have and bearer tokens do not, and it is why two settings stop being optional:

- **`CORS_ALLOW_ORIGIN` may not be `*`.** A wildcard says every site is this
  application's frontend. The application refuses to start with that combination,
  and browsers refuse `*` together with credentials in any case. A frontend served
  from this same origin needs no CORS at all — leave the key unset.
- **`CORS_ALLOW_CREDENTIALS=true`** when the frontend is on another origin, or the
  browser drops the cookie and every request arrives anonymous.

The same list feeds the cross-origin check in
[internal/middleware/csrf.go](internal/middleware/csrf.go), which refuses
state-changing requests a browser reports as coming from an untrusted origin.
It reads `CORS_ALLOW_ORIGIN` rather than a setting of its own, so the two cannot
disagree about which origin is yours.


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

> These cover the HTTP surface. For the message broker and WebSocket surface,
> see [Messaging documentation](#messaging-documentation) below.

### 仕組み

どちらのコマンドも、`cmd/apidoc-describe` を `go run -tags apidoc_describe` で実行し、
**リフレクションで API 仕様（型・検証ルール・ルーティング宣言）を書き出した中立モデル**を回収してから、
それぞれの形式にレンダリングします。

describe は build tag `apidoc_describe` で本番ビルドから隔離されており、
**DB やメッセージブローカーには接続しません**（接続系はスタブが注入され、fx のライフサイクルも起動しません）。
そのためインフラを立ち上げずにドキュメントを生成できます。

### Adding a dependency that describe has to stub

describe registers no real infrastructure. It builds the DI graph from the module
constructors plus one fixed list of stubs in
[internal/app/bootstrap/describe/stubs.go](internal/app/bootstrap/describe/stubs.go).
So when a module — a controller, service, repository, job or task — starts
injecting an infrastructure type that list does not carry, resolution fails, and
the generator names the type it could not build:

```
apidoc-describe: describe: resolve http modules: ... missing type: cache.Cache

describe has no stub for: cache.Cache
Add one for each to internal/app/bootstrap/describe/stubs.go
```

**Add a stub for the named type to `stubs.go`, in the same change that introduces
the dependency.** `apidoc-describe` and `msgdoc-describe` share that one list, so
a single entry covers both.

**A stub is never executed.** describe reads declarations: it runs no handler, job
or subscription, and it never starts the fx lifecycle. All a stub has to do is be
constructible, so an in-memory implementation from the library, a zero value, or a
no-op fake is enough — it does not have to behave like the real thing.

`stubs.go` carries every type `server.Run` or `scheduler.Start` provides that a
module can inject, whether or not a module injects one today. Carrying only what
is currently used is what left the hole in the first place: the answer to "does
anything inject this" changes the moment somebody adds a dependency, and `fx`
builds lazily, so nothing notices until some module actually reaches the type.

The specs in that package resolve both graphs, so a missing stub turns
`go test ./internal/app/bootstrap/describe/...` red as well. Without them the only
thing that broke was document generation — a path most work never touches, which
is why the last gap went unnoticed until somebody regenerated the documents.

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


## Messaging documentation

The messaging surface — what the application consumes from and publishes to a
message broker, and what flows over its WebSocket channels — is generated the
same way the HTTP surface is: from the implementation, with no annotations.

```sh
encli generate asyncapi    # AsyncAPI 3.0 (docs/asyncapi.yaml)
```

Every operation is written from this application's own point of view, so `send`
always means this application sends and `receive` that it consumes. Without that
fixed perspective, a channel with a producer and a consumer reads ambiguously.

`msgdoc-describe` resolves its declarations the same way `apidoc-describe` does,
from the same stub list — so when a module gains an infrastructure dependency, see
[Adding a dependency that describe has to stub](#adding-a-dependency-that-describe-has-to-stub).

### What it reads

Generation reflects over declarations, so a channel only appears in the document
if it was declared. Three kinds of declaration exist:

| Declaration | What it describes |
|---|---|
| `mbkit.Subscription[Msg]` | one broker subscription (a `receive` operation) |
| `mbkit.PublicationSpec[Msg]` | one broker publication (a `send` operation) |
| `wskit.Channel` | one WebSocket path and its message catalog, in both directions |

These are not documentation-only. A subscription's handler is reached through
its declaration, a publication is the only way the application publishes, and a
WebSocket message reaches application code only through its declared receiver.
Changing the code without the declaration does not compile, which is what keeps
the document from drifting away from what runs.

### The WebSocket envelope

A WebSocket path carries many kinds of message in both directions, and the frame
itself says nothing about which one it is. Every message is therefore wrapped in
a fixed envelope, which supplies that discriminator once:

```json
{"type": "user.echo", "data": {"message": "hi"}}
```

`wskit` parses it, routes the message to the receiver declared under that name,
validates the payload, and calls the handler with the decoded value. A message
that cannot be decoded, or whose type is not declared, is answered with an error
message and the connection stays open — one bad message from a client should not
cost it every other message on the socket.

### Where the facts come from

As with the HTTP documents, anything no type can express has to be declared.
A `TODO` in the output means the corresponding declaration is empty.

| Output | Declared in |
|---|---|
| Title, version, summary, license | [internal/app/apiinfo](internal/app/apiinfo/apiinfo.go) |
| Summary, description, field meanings, related operations | `Summary` / `Description` / `FieldDocs` / `Related` on the declaration |
| Side effects, idempotency, preconditions, scopes, ordering | `Behavior` on the declaration |
| Delivery guarantee | `Behavior.Delivery` on the mb declaration |
| Payload type, required fields, constraints | the type parameter and `BodyRules` |
| Broker and WebSocket endpoints | `BROKER_*` and `HTTP_PORT` in the environment configuration |

The delivery section carries two things: the guarantee the author declared, and
the settings the subscribe/publish options actually resolve to. Both are kept
because they can disagree — a document showing only the prose would hide a
subscription whose options contradict it.

### Not overwritten

Generated documents carry an `x-generated` marker, and a file without one is
treated as hand-written and never overwritten, exactly as for OpenAPI.

> `encli generate docai --messaging` (DocAI Messaging) is not supported yet, as
> that format is still a draft. Use `encli generate asyncapi` for the messaging
> surface in the meantime.


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

## The gRPC server

The gRPC server is built in [internal/app/grpc/grpc.go](internal/app/grpc/grpc.go)
and reads its settings from config, the same way the HTTP server does.

**There is no `GRPC_ENABLED`.** Whether this application serves gRPC is decided
by whether `server.Run` registers the gRPC app in its invocations — the same
switch the worker, the scheduler and the message broker have. A flag would be a
second answer to the same question, and the pair would eventually disagree:
`GRPC_ENABLED=true` with the invocation deleted is a setting that says the
server is on while nothing is listening. An application that does not serve gRPC
deletes the invocation, and then no port is opened either.

| Key | Default | What it does |
|---|---|---|
| `GRPC_PORT` | `50051` | The port the server listens on |
| `GRPC_REFLECTION` | unset | Three-state; see below |
| `GRPC_GRACEFUL_STOP_TIMEOUT` | unset | A gRPC-specific grace period for in-flight calls |
| `GRPC_MAX_RECV_MSG_SIZE` / `GRPC_MAX_SEND_MSG_SIZE` | unset | Message size caps, in bytes |
| `GRPC_KEEPALIVE_TIME` / `GRPC_KEEPALIVE_TIMEOUT` | unset | Server-side keepalive pings |
| `GRPC_KEEPALIVE_MIN_TIME` / `GRPC_KEEPALIVE_PERMIT_WITHOUT_STREAM` | unset / `false` | The client-side limits that go with them |
| `GRPC_LOG_SUCCESS_SAMPLE_RATE` | `0.3` | The share of *successful* calls that are logged, `0`–`1` |
| `GRPC_LOG_MAX_HEADER_VALUE_LEN` | `64` | The length a logged header value is truncated to |

An unset limit or keepalive duration leaves grpc-go's own default in place —
this template does not pick a message size for every application built from it.
`GRPC_GRACEFUL_STOP_TIMEOUT` unset means the shutdown is bounded by the
application's stop deadline, as it was before the setting existed; set it to
give gRPC a *shorter* grace period, so that a long-running stream cannot spend
the whole shutdown budget.

### `GRPC_REFLECTION` has three states

Reflection lets `grpcurl` and IDE clients discover the services this server
exposes without a `.proto` file, which makes it a development convenience and a
production exposure at once.

| Value | Meaning |
|---|---|
| unset | `local` and `development` serve it; everything else does not |
| `true` | Served, whatever the environment |
| `false` | Not served, whatever the environment |

The environment default is the safe one, and the setting overrules it in both
directions: on, to debug a staging deployment; off, for a development
deployment that is reachable from outside.

### What stays in code

`LogConfig` in [internal/app/grpc/log_config.go](internal/app/grpc/log_config.go)
keeps the header policy — which headers are logged, which are masked, what they
are masked with, which method prefixes are skipped. Those are this
application's rules: they are reviewed next to the services they describe, and
they do not change because a deployment moved. Only the two values a deployment
genuinely changes — how much successful traffic is logged, and how far a header
value is truncated — come from config.

`GRPC_LOG_SUCCESS_SAMPLE_RATE=0` means no successful call is logged; failures
still are. The interceptor reads a zero sample rate as "not configured" and
would log everything, so `NewGRPCServer` honors the setting by handing it
success loggers that write nothing.

TLS is not configured here. The server serves plaintext behind whatever
terminates TLS in front of it.

## Caching a read (`enscache.Cache`)

The application cache is a tiered store: a bounded in-process L1 (otter) over a
shared L2 (Redis), built in
[internal/infra/cache/cache.go](internal/infra/cache/cache.go) and injected as
`enscache.Cache`. `internal/query/user_post` is the worked example.

### The shape a read-through cache wants

```go
func (s *userPostService) GetByID(ctx context.Context, id int64) (*record.UserPostRecord, error) {
	return enscache.GetOrSetFunc(ctx, s.cache, cacheKey(id), cacheTTL,
		func(ctx context.Context) (*record.UserPostRecord, error) {
			return s.repository.GetByID(ctx, id)
		})
}
```

`GetOrSetFunc` returns the cached value on a hit, and otherwise runs the
function, stores what it returns and hands it back. The repository is therefore
consulted only on a miss, and only once per key even when several requests miss
at the same time.

An error from the function is returned as-is and **nothing is stored**, so one
unlucky moment never becomes the answer for as long as the entry would live.

Two things follow from adding the cache:

- **The method takes a `ctx`.** The read now reaches Redis, and a caller that
  goes away should stop the work it started.
- **The method returns an `error`.** The record itself may be incapable of
  failing, but the L2 is a network hop; a caller has to hear about a Redis that
  is unreachable rather than have it read through silently.

### Keys

A key has to name **every input the value depends on**. `GetByID` depends only
on the id, so that is all the key carries:

```go
const cacheKeyNamespace = "user_post"

func cacheKey(id int64) string {
	return fmt.Sprintf("%s:%d", cacheKeyNamespace, id)
}
```

A read that also varied by, say, a locale or by who is asking would have to put
those in the key too — otherwise two different answers overwrite each other and
one caller is served the other's.

The store prepends its own `cacheKeyPrefix` (`"app"`), so the key a Redis
instance actually holds is `app:user_post:42`. The namespace only has to be
unique among the modules of one application.

### Two different TTLs

| Setting | What it bounds |
|---|---|
| `cacheTTL` in the module | how stale the cached value may be — a question about the data |
| `CACHE_NEAR_TTL` | how long one replica's in-process L1 copy may differ from the shared L2 |

### Invalidation

This example is a read-only query module, so it lets entries expire. An
application that also writes has a better option: **delete the key where the
write happens.**

```go
// In the module that writes, after the write succeeds.
func (s *postService) Update(ctx context.Context, post *record.PostRecord) error {
	if err := s.repository.Update(ctx, post); err != nil {
		return err
	}

	// The reader would otherwise serve the old value until its TTL runs out.
	return s.cache.Delete(ctx, cacheKey(post.ID))
}
```

Two things make this work, and both are easy to get wrong:

- **The writer and the reader have to build the key the same way.** They are
  usually in different modules, so the key builder belongs somewhere both can
  reach rather than copied into each.
- **Delete after the write succeeds, not before.** Deleting first leaves a
  window where a read can repopulate the entry from the old data and then the
  write lands, leaving a cache that disagrees with the database until the TTL
  expires.

A TTL is still worth setting even with invalidation in place: it is what bounds
the damage when a delete is missed.

### Why anything is injected at all

`fx` builds lazily, so a constructor nothing depends on never runs. Before this
example existed, `NewDefaultCache` was provided but unused — which meant a
misconfigured cache Redis stayed invisible until somebody wrote the first cached
read. With the injection in place, startup dials it and says so:

```
{"level":"INFO","msg":"Redis connection verified","purpose":"cache","addr":"localhost","db":2}
```

### Inject `enscache.Cache`, not the Redis client

A module can reach the raw `*redis.Client` as well — `server.Run` provides the
worker's client unnamed — but a module that injects it will run under `server` and
fail to start under `scheduler`:

| | `*redis.Client` | `enscache.Cache` |
|---|---|---|
| `server.Run` | provided unnamed | provided |
| `scheduler.Start` | only named: `workerCache`, `schedulerCache` | provided |

`scheduler.Start` registers both of its clients through `dikit.ProvideNamed`, so
an unnamed `*redis.Client` has no provider there and the scheduler dies at startup
with `missing type: *redis.Client` — while the HTTP application, built from the
same modules, has been running fine the whole time.

Inject `enscache.Cache`. Both applications provide it unnamed, it is the
abstraction the codec and the L1/L2 tiering live behind, and it leaves the raw
handle and its connection lifecycle owned by one place.

describe stubs `*redis.Client` too, so document generation will not catch this for
you: it fails at scheduler startup, not at generation time.


## .envファイルの注意事項

`.env`ファイルは、ローカル環境、テスト環境でのみ利用することが想定されています。
それ以外の環境が、`.env`の値を使うことを想定せずに実装してください。

特に、`encli build migration`で出力する設定ファイルには、`.env`は含まれません。




## Failing at startup

Everything the application assembles at startup runs inside fx, and fx has one
way of being told that something is wrong: a returned `error`. Constructors and
invocations use it. **Do not call `log.Fatal`, `panic` or `os.Exit` from a
constructor or an invocation.**

```go
// Constructor: it produces a value, so it returns (T, error).
func NewThing(lc dikit.LC) (*Thing, error) {
	params, err := registry.ModuleParams("default")
	if err != nil {
		return nil, fmt.Errorf("default config parameters not found: %w", err)
	}
	...
}

// Invocation: it may produce a value or nothing, and returns an error either way.
func StartThing(thing *Thing) error { ... }
```

The reason is what an operator sees when the application refuses to start.
`dikit.ProvideAndRun` hands the failure back to `main`, which writes exactly one
structured record through the configured logger and exits 1:

```json
{"level":"ERROR","msg":"server exited abnormally","error":"... : dial tcp [::1]:5432: connect: connection refused"}
```

`log.Fatal` would write an unstructured line to stderr instead, outside the log
setup, and its `os.Exit` would cut the shutdown short. Writing the reason only
through fx's own event log is no better: that log is on solely when
`LOG_LEVEL=debug`, so in every other environment the failure would be silent.

The same applies to a startup check. An `OnStart` hook that cannot reach what it
needs returns an error, and fx rolls back the hooks that had already started:

```go
lc.Append(dikit.Hook{
	OnStart: func(ctx context.Context) error {
		if err := conn.Ping(ctx); err != nil {
			return fmt.Errorf("cache connection check failed: %w", err)
		}
		return nil
	},
})
```

### A server that dies after it has started

Once the application is running, a long-lived server that dies is not a startup
failure and has no error to return — it is running in its own goroutine. It logs
why and asks for shutdown **with an exit code**:

```go
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	loggear.Error("HTTP server stopped unexpectedly", "error", err)
	_ = shutdowner.Shutdown(dikit.ExitCode(1))
}
```

`dikit.ExitCode` is what makes the process exit non-zero. Without it the shutdown
is indistinguishable from a clean one: the process ends with 0, and a supervisor
set to restart on failure (systemd's `Restart=on-failure`, a Kubernetes
`restartPolicy`) reads that as a deliberate stop and leaves the application down.

The result is two records — the reason, and the exit:

```json
{"level":"ERROR","msg":"HTTP server stopped unexpectedly","error":"listen tcp :8080: bind: address already in use"}
{"level":"INFO","msg":"server exited with requested exit code","code":1}
```

The error record is yours to write: `main` only reports *that* the application
asked to exit with a code, never *why*.
