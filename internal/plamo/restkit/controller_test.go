package restkit_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

type createReq struct {
	Name string `json:"name"`
}

type createRes struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// teapotError は HTTPError を実装したテスト用エラー。
type teapotError struct{}

func (teapotError) Error() string { return "i am a teapot" }
func (teapotError) Status() int   { return http.StatusTeapot }
func (teapotError) Code() string  { return "teapot" }

func jsonRequest(body string) *rest.Request {
	return jsonRequestLang(body, "")
}

func jsonRequestLang(body, lang string) *rest.Request {
	httpReq := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if lang != "" {
		httpReq.Header.Set("Accept-Language", lang)
	}
	return rest.NewRequest(httpReq)
}

var _ = Describe("endpoint controller", func() {
	// name は最大5文字という制約付きの作成エンドポイント。
	newEndpoint := func(handle func(r *rest.Request, req *createReq) (*rest.Result[createRes], error)) *restkit.Endpoint[createReq, createRes] {
		return &restkit.Endpoint[createReq, createRes]{
			Success:   http.StatusCreated,
			BodyRules: []*rule.RuleSet{{Field: "name", Rules: []rule.Rule{vkit.MaxLength(5)}}},
			Handle:    handle,
		}
	}

	okHandle := func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
		return rest.NewResult(&createRes{ID: "usr_01", Name: req.Name}), nil
	}

	Describe("success path", func() {
		It("validates, calls the handler, and applies the declared Success status", func() {
			ctrl := restkit.NewController(newEndpoint(okHandle))

			res := ctrl.Handle(jsonRequest(`{"name":"Taro"}`))

			Expect(res.Code).To(Equal(http.StatusCreated))
			body, ok := res.Body.(*createRes)
			Expect(ok).To(BeTrue())
			Expect(body.Name).To(Equal("Taro"))
		})

		It("lets the handler override the status via Result", func() {
			ep := newEndpoint(func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
				return rest.NewResult(&createRes{ID: "usr_01"}).WithStatus(http.StatusAccepted), nil
			})
			res := restkit.NewController(ep).Handle(jsonRequest(`{"name":"Taro"}`))

			Expect(res.Code).To(Equal(http.StatusAccepted))
		})

		It("pins ContentType from Produces when the result did not set one", func() {
			ep := newEndpoint(okHandle)
			ep.Produces = rest.MediaTypeXML
			res := restkit.NewController(ep).Handle(jsonRequest(`{"name":"Taro"}`))

			Expect(res.ContentType).To(Equal(rest.MediaTypeXML))
		})
	})

	Describe("validation", func() {
		It("renders the docai error envelope with field_errors on a body rule failure", func() {
			ctrl := restkit.NewController(newEndpoint(okHandle))

			res := ctrl.Handle(jsonRequest(`{"name":"tooLongName"}`))

			Expect(res.Code).To(Equal(http.StatusUnprocessableEntity))
			env, ok := res.Body.(*restkit.ErrorEnvelope)
			Expect(ok).To(BeTrue())
			Expect(env.Error.Code).To(Equal("validation_failed"))
			Expect(env.Error.FieldErrors).To(HaveLen(1))
			Expect(env.Error.FieldErrors[0].Field).To(Equal("name"))
			Expect(env.Error.FieldErrors[0].Code).To(Equal("str_max_length"))
			Expect(env.Error.FieldErrors[0].Message).To(ContainSubstring("exceeds maximum length"))
		})

		It("selects the display language from Accept-Language", func() {
			res := restkit.NewController(newEndpoint(okHandle)).
				Handle(jsonRequestLang(`{"name":"tooLongName"}`, "ja-JP,ja;q=0.9"))

			env := res.Body.(*restkit.ErrorEnvelope)
			Expect(env.Error.FieldErrors[0].Message).To(ContainSubstring("最大文字数"))
		})

		It("returns 400 not_parsable for malformed JSON, without field_errors", func() {
			res := restkit.NewController(newEndpoint(okHandle)).Handle(jsonRequest(`{not json`))

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			env := res.Body.(*restkit.ErrorEnvelope)
			Expect(env.Error.Code).To(Equal("not_parsable"))
			Expect(env.Error.FieldErrors).To(BeEmpty())
		})

		It("does not invoke the handler when validation fails", func() {
			called := false
			ctrl := restkit.NewController(newEndpoint(func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
				called = true
				return rest.NewResult(&createRes{}), nil
			}))

			ctrl.Handle(jsonRequest(`{"name":"tooLongName"}`))

			Expect(called).To(BeFalse())
		})
	})

	Describe("error mapping", func() {
		It("uses status and code from an HTTPError", func() {
			ctrl := restkit.NewController(newEndpoint(func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
				return nil, teapotError{}
			}))

			res := ctrl.Handle(jsonRequest(`{"name":"Taro"}`))

			Expect(res.Code).To(Equal(http.StatusTeapot))
			env, ok := res.Body.(*restkit.ErrorEnvelope)
			Expect(ok).To(BeTrue())
			Expect(env.Error.Code).To(Equal("teapot"))
			Expect(env.Error.Message).To(Equal("i am a teapot"))
		})

		It("collapses an unknown error to 500 without leaking details", func() {
			ctrl := restkit.NewController(newEndpoint(func(r *rest.Request, req *createReq) (*rest.Result[createRes], error) {
				return nil, errorf("db exploded: secret connection string")
			}))

			res := ctrl.Handle(jsonRequest(`{"name":"Taro"}`))

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
			env := res.Body.(*restkit.ErrorEnvelope)
			Expect(env.Error.Code).To(Equal("internal_error"))
			Expect(env.Error.Message).NotTo(ContainSubstring("secret"))
		})
	})

	Describe("EndpointDoc", func() {
		It("exposes the request/response types and declared metadata", func() {
			ep := newEndpoint(okHandle)
			ep.Summary = "Create a user"
			ctrl := restkit.NewController(ep)

			doc := ctrl.(restkit.Documented).EndpointDoc()

			Expect(doc.Summary).To(Equal("Create a user"))
			Expect(doc.Success).To(Equal(http.StatusCreated))
			Expect(doc.ReqType).To(Equal(reflect.TypeFor[createReq]()))
			Expect(doc.ResType).To(Equal(reflect.TypeFor[createRes]()))
			Expect(doc.BodyRules).To(HaveLen(1))
			Expect(doc.BodyRules[0].Field).To(Equal("name"))
		})
	})
})

// errorf は fmt を持ち込まずに簡単な error を作るテスト用ヘルパー。
type simpleError string

func (e simpleError) Error() string { return string(e) }

func errorf(msg string) error { return simpleError(msg) }
