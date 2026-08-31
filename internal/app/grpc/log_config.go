package grpc

import (
	"sort"
	"strings"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/grpcgear/pkg/interceptor/logging/logsrv"
	"github.com/ensoria/loggear/pkg/loggear"
	"google.golang.org/grpc/codes"
)

// LogConfig builds the gRPC logging settings.
//
// The split between what is written here and what comes from config is "does a
// deployment change it": how much of the successful traffic is logged and how
// far a header value is truncated differ between a service taking a handful of
// calls and the same service taking thousands, so they are settings. Which
// headers are read, which are masked, what they are masked with and which
// methods are skipped are this application's policy — they are edited here, in
// code that is type-checked and reviewed next to the services it describes, and
// they do not change because a deployment moved.
func LogConfig(config *appconfig.GRPC) *logsrv.ServerConfig {
	cfg := &logsrv.ServerConfig{
		RequestIDKey:    "x-request-id",
		TraceIDKey:      "x-trace-id",
		RetryAttemptKey: "x-retry-attempt",
		IncludeHeaders:  []string{"user-agent", "grpc-accept-encoding", "x-user-id"},
		ExcludeHeaders:  []string{"grpc-version"},
		SensitiveHeaders: []string{
			"authorization",
			"x-api-key",
			"cookie",
			"x-user-id",
		},
		MaskWith: "***",
	}

	cfg.AutoLevel = true
	cfg.SuccessSampleRate = config.LogSuccessSampleRate
	cfg.MaxHeaderValueLen = config.LogMaxHeaderValueLen
	cfg.SkipMethodPrefixes = []string{"/grpc.health.v1."}

	return cfg
}

func CreateBasicUnaryLogFuncs(logger loggear.Logger) (logsrv.LogUnarySuccessFunc, logsrv.LogUnaryErrorFunc) {
	build := func(info *logsrv.UnaryInfo) []any {
		attrs := baseCommonAttrs(info.Code, info.DurationMS, info.Server.FullMethod, info.Client)
		if info.Err != nil {
			attrs = append(attrs, "error", info.Err.Error())
		}
		return attrs
	}

	logSuccess := func(info *logsrv.UnaryInfo) {
		logger.Info("gRPC unary", build(info)...) // 成否は code と error 有無で判別
	}
	logError := func(info *logsrv.UnaryInfo) {
		logger.Error("gRPC unary", build(info)...)
	}
	return logSuccess, logError
}

func CreateBasicStreamLogFuncs(logger loggear.Logger) (logsrv.LogStreamSuccessFunc, logsrv.LogStreamErrorFunc) {
	build := func(info *logsrv.StreamInfo) []any {
		attrs := baseCommonAttrs(info.Code, info.DurationMS, info.Server.FullMethod, info.Client)
		// ストリーム特有のフラグ
		attrs = append(attrs, "client_stream", info.Server.IsClientStream, "server_stream", info.Server.IsServerStream)
		if info.Err != nil {
			attrs = append(attrs, "error", info.Err.Error())
		}
		return attrs
	}
	logSuccess := func(info *logsrv.StreamInfo) {
		logger.Info("gRPC stream", build(info)...) // code と error で判別
	}
	logError := func(info *logsrv.StreamInfo) {
		logger.Error("gRPC stream", build(info)...)
	}
	return logSuccess, logError
}

// arrangeStreamLogInfo の unary 版
func baseCommonAttrs(code codes.Code, durationMS int64, fullMethod string, client map[string]any) []any {
	attrs := []any{
		"method", fullMethod,
		"code", code,
		"duration_ms", durationMS,
	}

	// 昇格させたい代表キー (存在すれば順序保証して追加)
	promote := []string{
		"request_id", "trace_id", "span_id", "retry_attempt", "client_ip", "client_port", "network",
		"deadline_remaining_ms", "deadline_unix", "trace_sampled", "auth_type",
	}
	seen := make(map[string]struct{})
	for _, k := range promote {
		if v, ok := client[k]; ok {
			attrs = append(attrs, k, v)
			seen[k] = struct{}{}
		}
	}
	// user-agent は key 名を user_agent に変換
	if ua, ok := client["user-agent"]; ok {
		attrs = append(attrs, "user_agent", ua)
		seen["user-agent"] = struct{}{}
	}

	// 残りのメタデータ (昇格済みを除外) をソートして付与。順序を安定化。
	remainingKeys := make([]string, 0, len(client))
	for k := range client {
		if _, ok := seen[k]; ok {
			continue
		}
		// すでに attrs に直接入っている昇格キー以外を対象
		remainingKeys = append(remainingKeys, k)
	}
	// ノイズ抑止: method/code/duration_ms と重複しないように安全フィルタ
	filterOut := map[string]struct{}{"method": {}, "code": {}, "duration_ms": {}}
	sort.Strings(remainingKeys)
	for _, k := range remainingKeys {
		if _, skip := filterOut[k]; skip {
			continue
		}
		v := client[k]
		canonicalKey := strings.ReplaceAll(k, "-", "_") // 出力の統一 (任意)
		if canonicalKey == "user_agent" {               // 既に追加済み
			continue
		}
		attrs = append(attrs, canonicalKey, v)
	}
	return attrs
}
