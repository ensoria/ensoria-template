# syntax=docker/dockerfile:1

# Ensoria アプリケーションのコンテナイメージ（ECS/EKS 等向け）。
# config の YAML は go:embed でバイナリに埋め込まれるため、ランタイムには YAML ファイルは不要。
# encli build image が `--build-arg MAIN_PATH=<対象>` を渡してビルド対象を切り替える。
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
# alpine には git が無い。private な間の ensoria 系モジュールは VCS 直 fetch のため git が要る。
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# private モジュール解決用。空（= 全モジュール公開後）なら go は proxy 経由で取得する。
ARG GOPRIVATE
ENV GOPRIVATE=${GOPRIVATE}

# 依存だけ先に取得してレイヤキャッシュを効かせる
COPY go.mod go.sum ./
# build secret `gh_token` があれば GitHub 認証を設定して private モジュールを取得する。
# secret は tmpfs でレイヤに残らず、トークン入り git 設定は同一 RUN 内で破棄する
# （最終 distroless ステージにはバイナリのみコピーするため非混入）。
# secret 未指定（= 公開後）なら認証はスキップし proxy 経由で取得する。
RUN --mount=type=secret,id=gh_token,required=false \
    sh -c 'set -e; \
      if [ -s /run/secrets/gh_token ]; then \
        export GIT_CONFIG_GLOBAL=/tmp/gitconfig; \
        git config --file "$GIT_CONFIG_GLOBAL" \
          url."https://x-access-token:$(cat /run/secrets/gh_token)@github.com/".insteadOf "https://github.com/"; \
      fi; \
      go mod download; \
      rm -f /tmp/gitconfig'

COPY . .

# ビルド対象（既定は server）。encli が --build-arg で上書きする。
ARG MAIN_PATH=./cmd/server
# TARGETOS/TARGETARCH は buildkit が --platform から自動設定する。
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /out/app ${MAIN_PATH}

# 静的バイナリ + CA 証明書同梱の最小ランタイム（nonroot）。
FROM gcr.io/distroless/static:nonroot AS runtime
COPY --from=builder /out/app /app
USER nonroot:nonroot
# 環境（development/staging/production）は実行時に --env で指定する。
# 例: docker run <image> --env production
ENTRYPOINT ["/app"]
