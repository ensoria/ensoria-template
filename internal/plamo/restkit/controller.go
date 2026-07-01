package restkit

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/validator/pkg/verr"
)

const (
	// validationErrorStatus は入力検証に失敗したときのステータス(docai 準拠: 422)。
	validationErrorStatus = http.StatusUnprocessableEntity
	// validationFailedCode / validationFailedMessage は検証エラーエンベロープの既定コード/文言。
	validationFailedCode    = "validation_failed"
	validationFailedMessage = "input is invalid"
	// parseErrorCode は verr.ParseError が用いるリクエスト全体エラーのコード。
	parseErrorCode = "not_parsable"
	// internalErrorMessage はハンドラが型不明のエラーを返したときの公開メッセージ。
	internalErrorMessage = "internal server error"
	// internalErrorCode は上記に対応する機械判定用コード。
	internalErrorCode = "internal_error"
	// defaultLang は Accept-Language が無い/未対応のときの表示言語。
	defaultLang = "en"
)

// HTTPError はハンドラが返すエラーのうち、ステータス/コードを自ら決められるもの。
// これを実装したエラーは、そのステータス・コードでクライアントに返される。
// 実装しないエラーは 500 internal_error に丸められる(内部詳細を漏らさない)。
type HTTPError interface {
	error
	Status() int
	Code() string
}

// ErrorEnvelope は docai のエラー本文形 `{"error": {...}}` に対応する。
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail はエラーのコード・表示メッセージと、任意のフィールド単位エラー。
type ErrorDetail struct {
	Code        string             `json:"code"`
	Message     string             `json:"message"`
	FieldErrors []FieldErrorDetail `json:"field_errors,omitempty"`
}

// FieldErrorDetail は docai の field_errors の1要素(表示言語1つに絞ったもの)。
type FieldErrorDetail struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewController は Endpoint を rest.Controller に適合させたコントローラを返す。
// rest.Module の Get/Post/... にそのまま代入できる。
func NewController[Req any, Res any](ep *Endpoint[Req, Res]) rest.Controller {
	return &endpointController[Req, Res]{ep: ep}
}

// endpointController は Endpoint をラップして rest.Controller と Documented を満たす。
type endpointController[Req any, Res any] struct {
	ep *Endpoint[Req, Res]
}

// Handle は検証 → ハンドラ呼び出し → Result 変換の順で処理する。
func (c *endpointController[Req, Res]) Handle(r *rest.Request) *rest.Response {
	lang := displayLang(r)

	// 1. リクエストボディの解析 + 検証
	req, vErrs := vkit.RestRequestBody[Req](r, c.ep.BodyRules...)
	if vErrs.HasErrors() {
		return validationErrorResponse(vErrs, lang)
	}

	// 2. パス/クエリパラメータの検証
	if vErrs := validatePathParams(r, c.ep.PathRules); vErrs.HasErrors() {
		return validationErrorResponse(vErrs, lang)
	}
	if vErrs := validateQueryParams(r, c.ep.QueryRules); vErrs.HasErrors() {
		return validationErrorResponse(vErrs, lang)
	}

	// 3. ハンドラ実行
	result, err := c.ep.Handle(r, req)
	if err != nil {
		return errorResponse(err)
	}

	// 4. 成功レスポンス(Produces が指定されていれば形式を固定)
	res := result.ToResponse(c.ep.Success)
	if res.ContentType == "" && c.ep.Produces != "" {
		res.ContentType = c.ep.Produces
	}
	return res
}

// EndpointDoc は apidoc 向けに、型情報と宣言メタを非ジェネリックな形で公開する。
func (c *endpointController[Req, Res]) EndpointDoc() EndpointDoc {
	return EndpointDoc{
		Summary:         c.ep.Summary,
		Description:     c.ep.Description,
		FieldDocs:       c.ep.FieldDocs,
		ReqType:         reflect.TypeFor[Req](),
		ResType:         reflect.TypeFor[Res](),
		BodyRules:       c.ep.BodyRules,
		PathRules:       c.ep.PathRules,
		QueryRules:      c.ep.QueryRules,
		Success:         c.ep.Success,
		ResponseHeaders: c.ep.ResponseHeaders,
		Produces:        c.ep.Produces,
		Responses:       c.ep.Responses,
		Errors:          c.ep.Errors,
		Behavior:        c.ep.Behavior,
	}
}

// validatePathParams は PathRules の各フィールドを r.PathValue から取り出して検証する。
func validatePathParams(r *rest.Request, rules []*rule.RuleSet) verr.ValidationErrors {
	if len(rules) == 0 {
		return nil
	}
	values := make(map[string]any, len(rules))
	for _, rs := range rules {
		v, _ := r.PathValue(rs.Field)
		values[rs.Field] = v
	}
	return vkit.Map(values, rules...)
}

// validateQueryParams は QueryRules の各フィールドを r.Query から取り出して検証する。
func validateQueryParams(r *rest.Request, rules []*rule.RuleSet) verr.ValidationErrors {
	if len(rules) == 0 {
		return nil
	}
	values := make(map[string]any, len(rules))
	for _, rs := range rules {
		v, _ := r.Query(rs.Field)
		values[rs.Field] = v
	}
	return vkit.Map(values, rules...)
}

// validationErrorResponse は中立形 verr.ValidationErrors を docai のエラーエンベロープに整形する。
// 表示言語は lang で1つに絞る(多言語は verr 側が保持しているので情報は失われない)。
// Field が空のリクエスト全体エラー(パース失敗)は field_errors ではなく top-level に載せる。
func validationErrorResponse(vErrs verr.ValidationErrors, lang string) *rest.Response {
	detail := ErrorDetail{Code: validationFailedCode, Message: validationFailedMessage}
	status := validationErrorStatus

	for _, fe := range vErrs {
		msg := pickMessage(fe.Messages, lang)
		if fe.Field == "" {
			// リクエスト全体エラー(例: JSON パース失敗)は top-level メッセージにする
			detail.Code = fe.Code
			detail.Message = msg
			if fe.Code == parseErrorCode {
				status = http.StatusBadRequest
			}
			continue
		}
		detail.FieldErrors = append(detail.FieldErrors, FieldErrorDetail{
			Field:   fe.Field,
			Code:    fe.Code,
			Message: msg,
		})
	}

	return &rest.Response{
		Code: status,
		Body: &ErrorEnvelope{Error: detail},
	}
}

// displayLang は Accept-Language の先頭の言語サブタグ(小文字)を返す。無ければ defaultLang。
// 例: "ja-JP,ja;q=0.9,en" -> "ja"
func displayLang(r *rest.Request) string {
	header, ok := r.Header("Accept-Language")
	if !ok || header == "" {
		return defaultLang
	}
	first := header
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, '-'); i >= 0 {
		first = first[:i]
	}
	first = strings.ToLower(strings.TrimSpace(first))
	if first == "" {
		return defaultLang
	}
	return first
}

// pickMessage は messages から lang のメッセージを選ぶ。無ければ defaultLang、
// それも無ければ任意の1つを返す。
func pickMessage(messages map[string]string, lang string) string {
	if msg, ok := messages[lang]; ok {
		return msg
	}
	if msg, ok := messages[defaultLang]; ok {
		return msg
	}
	for _, msg := range messages {
		return msg
	}
	return ""
}

// errorResponse はハンドラが返したエラーをレスポンスに変換する。
// HTTPError を実装していればそのステータス/コード、そうでなければ 500 に丸める。
func errorResponse(err error) *rest.Response {
	if he, ok := err.(HTTPError); ok {
		return &rest.Response{
			Code: he.Status(),
			Body: &ErrorEnvelope{Error: ErrorDetail{Code: he.Code(), Message: he.Error()}},
		}
	}
	return &rest.Response{
		Code: http.StatusInternalServerError,
		Body: &ErrorEnvelope{Error: ErrorDetail{Code: internalErrorCode, Message: internalErrorMessage}},
	}
}
