package vkit

import (
	"sort"
	"strconv"
	"strings"
)

// DefaultLang is the display language used when the caller asked for none, or
// for none the application has messages in.
const DefaultLang = "en"

// PreferredLangs parses an Accept-Language header into language subtags in
// descending q order, always ending with DefaultLang as the final fallback.
//
//	"fr;q=0.8, ja-JP, en;q=0.9" -> ["ja", "en", "fr", "en"]
//
// It lives here, rather than in one transport's kit, because validation errors
// carry every language and each transport has to pick one: HTTP reads the
// request header, WebSocket the header from the upgrade handshake. Duplicating
// the parsing would let the two disagree about which language a caller gets.
func PreferredLangs(header string) []string {
	return append(parseAcceptLanguage(header), DefaultLang)
}

// PickMessage returns the message for the first language available, falling back
// to any message at all rather than to an empty string — a message in the wrong
// language still tells the caller what went wrong.
func PickMessage(messages map[string]string, langs []string) string {
	for _, lang := range langs {
		if msg, ok := messages[lang]; ok {
			return msg
		}
	}
	for _, msg := range messages {
		return msg
	}
	return ""
}

// parseAcceptLanguage returns the language subtags (lowercased, deduplicated) in
// descending q order, ties keeping their order of appearance. `*` and q=0 are
// dropped.
func parseAcceptLanguage(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}

	type langQ struct {
		lang string
		q    float64
	}
	var entries []langQ
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lang := part
		q := 1.0
		if semi := strings.IndexByte(part, ';'); semi >= 0 {
			lang = strings.TrimSpace(part[:semi])
			for _, p := range strings.Split(part[semi+1:], ";") {
				if value, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
					if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
						q = parsed
					}
				}
			}
		}
		// 一次サブタグ(例: "ja-JP" -> "ja")に正規化
		if dash := strings.IndexByte(lang, '-'); dash >= 0 {
			lang = lang[:dash]
		}
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang == "" || lang == "*" || q <= 0 {
			continue
		}
		entries = append(entries, langQ{lang, q})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].q > entries[j].q
	})

	seen := make(map[string]bool, len(entries))
	langs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !seen[e.lang] {
			seen[e.lang] = true
			langs = append(langs, e.lang)
		}
	}
	return langs
}
