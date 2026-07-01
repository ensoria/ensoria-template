package restkit

import (
	"net/http"
	"reflect"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/validator/pkg/verr"
)

const (
	// validationErrorStatus は入力検証に失敗したときのステータス(docai 準拠: 422)。
	validationErrorStatus = http.StatusUnprocessableEntity
	// internalErrorMessage はハンドラが型不明のエラーを返したときの公開メッセージ。
	internalErrorMessage = "internal server error"
	// internalErrorCode は上記に対応する機械判定用コード。
	internalErrorCode = "internal_error"
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

// ErrorDetail はエラーのコードと表示メッセージ。
type ErrorDetail struct {
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
	// 1. リクエストボディの解析 + 検証
	req, vErrs := vkit.RestRequestBody[Req](r, c.ep.BodyRules...)
	if vErrs != nil {
		return validationErrorResponse(vErrs)
	}

	// 2. パス/クエリパラメータの検証
	if vErrs := validatePathParams(r, c.ep.PathRules); vErrs != nil {
		return validationErrorResponse(vErrs)
	}
	if vErrs := validateQueryParams(r, c.ep.QueryRules); vErrs != nil {
		return validationErrorResponse(vErrs)
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
func validatePathParams(r *rest.Request, rules []*rule.RuleSet) verr.ValidationErrorMessages {
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
func validateQueryParams(r *rest.Request, rules []*rule.RuleSet) verr.ValidationErrorMessages {
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

// validationErrorResponse はフィールド単位の検証エラーをそのまま本文に載せて返す。
func validationErrorResponse(vErrs verr.ValidationErrorMessages) *rest.Response {
	return &rest.Response{
		Code: validationErrorStatus,
		Body: vErrs,
	}
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
