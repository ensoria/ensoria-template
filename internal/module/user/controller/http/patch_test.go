package http_test

import (
	nethttp "net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	userhttp "github.com/ensoria/ensoria-template/internal/module/user/controller/http"
	"github.com/ensoria/ensoria-template/internal/module/user/service/mock"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// writer holds the scope the update endpoint requires.
func writer() *authkit.Principal {
	return &authkit.Principal{
		Subject: "usr_1",
		Scheme:  authkit.SchemeJWT,
		Scopes:  []string{"users:write"},
	}
}

// patch sends a partial update for user usr_1.
func patch(body string, caller *authkit.Principal) *rest.Response {
	ctrl := restkit.NewController(userhttp.NewPatch(&mock.UserServiceMock{}))

	raw := httptest.NewRequest(nethttp.MethodPatch, "/users/usr_1", strings.NewReader(body))
	raw.Header.Set("Content-Type", "application/json")
	raw.SetPathValue("id", "usr_1")
	r := rest.NewRequest(raw)
	if caller != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), caller))
	}
	return ctrl.Handle(r)
}

// fieldErrors reads the field-level errors out of the shared error envelope.
func fieldErrors(res *rest.Response) []restkit.FieldErrorDetail {
	GinkgoHelper()
	envelope, ok := res.Body.(*restkit.ErrorEnvelope)
	Expect(ok).To(BeTrue(), "the response body is not the shared error envelope")
	return envelope.Error.FieldErrors
}

var _ = Describe("PATCH /users/{id}", func() {
	It("refuses a caller with no credential", func() {
		Expect(patch(`{"name":"taro"}`, nil).Code).To(Equal(nethttp.StatusUnauthorized))
	})

	// An empty patch is a request to change nothing, which is valid.
	It("accepts a body that touches no field", func() {
		Expect(patch(`{}`, writer()).Code).To(Equal(nethttp.StatusOK))
	})

	It("accepts a field that is given a new value", func() {
		Expect(patch(`{"name":"taro"}`, writer()).Code).To(Equal(nethttp.StatusOK))
	})

	// Leaving a field out and clearing it are different requests. name allows
	// the first and refuses the second — a distinction a pointer cannot make.
	It("refuses clearing a field that may not be cleared", func() {
		res := patch(`{"name":null}`, writer())

		Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
		Expect(fieldErrors(res)).To(HaveLen(1))
		Expect(fieldErrors(res)[0].Field).To(Equal("name"))
		Expect(fieldErrors(res)[0].Code).To(Equal("not_null_if_set"))
	})

	It("allows clearing a field that has no such rule", func() {
		Expect(patch(`{"nickname":null}`, writer()).Code).To(Equal(nethttp.StatusOK))
	})

	// A constraint applies to a value that was given, and says nothing about a
	// field nobody mentioned.
	It("constrains a field that was given", func() {
		res := patch(`{"name":"a-very-long-name"}`, writer())

		Expect(res.Code).To(Equal(nethttp.StatusUnprocessableEntity))
		Expect(fieldErrors(res)[0].Code).To(Equal("str_max_length"))
	})

	It("does not constrain a field that was left out", func() {
		Expect(patch(`{"nickname":"taro"}`, writer()).Code).To(Equal(nethttp.StatusOK))
	})

	// Reading a field that was not given must not overwrite anything: the
	// handler has to ask whether it was set, not just take its value.
	It("leaves a field alone when the request does not mention it", func() {
		res := patch(`{"nickname":"taro"}`, writer())

		Expect(res.Body).To(HaveField("Name", Equal("hoge")))
	})

	It("applies a field that was given", func() {
		res := patch(`{"name":"taro"}`, writer())

		Expect(res.Body).To(HaveField("Name", Equal("taro")))
	})
})
