## configからまだ設定値を読み込んでいない箇所

設定値を`config`の読み込みができていないところを、`config`から読むようにする。

workerDBやcache、mbなど、まだconfigから取得せず、固定値が入力されているので、このあたりを固定値ではなくconfigから取得するように修正。TODO: のラベルが入ってるので、そこから探すこと

- `ensoria-template/internal/infra/db/db.go`
- `ensoria-template/internal/infra/mb/conn.go`
- `ensoria-template/internal/infra/cache/cache.go`