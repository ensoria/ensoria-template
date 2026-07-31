package restkit_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// guarded builds a controller with the given security declaration (test helper).
func guarded(security *restkit.SecuritySpec) rest.Controller {
	ep := &restkit.Endpoint[restkit.NoBody, okBody]{
		Security: security,
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[okBody], error) {
			return rest.NewResult(&okBody{OK: true}), nil
		},
	}
	return restkit.NewController(ep)
}

// callerRequest builds a request already carrying a verified caller, the way the
// authentication middleware leaves it (test helper).
func callerRequest(p *authkit.Principal) *rest.Request {
	r := rest.NewRequest(httptest.NewRequest(http.MethodGet, "/things", nil))
	if p != nil {
		r.SetContext(authkit.WithPrincipal(r.Context(), p))
	}
	return r
}

var _ = Describe("endpoint security", func() {
	jwtCaller := func(scopes ...string) *authkit.Principal {
		return &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT, Scopes: scopes}
	}

	// Fail-closed: an endpoint that declares nothing needs a caller. Forgetting
	// the declaration must close the endpoint, never open it.
	Describe("an endpoint with no declaration", func() {
		It("refuses an anonymous caller", func() {
			res := guarded(nil).Handle(callerRequest(nil))

			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})

		It("serves a verified caller", func() {
			res := guarded(nil).Handle(callerRequest(jwtCaller()))

			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("an endpoint declared public", func() {
		It("serves an anonymous caller", func() {
			res := guarded(&restkit.SecuritySpec{Public: true}).Handle(callerRequest(nil))

			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("still serves a caller that happens to be authenticated", func() {
			res := guarded(&restkit.SecuritySpec{Public: true}).Handle(callerRequest(jwtCaller()))

			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("required scopes", func() {
		writes := &restkit.SecuritySpec{Scopes: []string{"users:write"}}

		It("serves a caller holding every required scope", func() {
			res := guarded(writes).Handle(callerRequest(jwtCaller("users:read", "users:write")))

			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("refuses a caller missing one", func() {
			res := guarded(writes).Handle(callerRequest(jwtCaller("users:read")))

			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		// Authentication and authorization are different answers: 401 says "say
		// who you are", 403 says "you are known and still may not".
		It("answers 401 rather than 403 when nobody is authenticated", func() {
			res := guarded(writes).Handle(callerRequest(nil))

			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("accepted schemes", func() {
		jwtOnly := &restkit.SecuritySpec{Schemes: []string{authkit.SchemeJWT}}

		It("serves a caller using an accepted scheme", func() {
			res := guarded(jwtOnly).Handle(callerRequest(jwtCaller()))

			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("refuses a caller using another scheme", func() {
			apiKeyCaller := &authkit.Principal{Subject: "svc_1", Scheme: authkit.SchemeAPIKey}

			res := guarded(jwtOnly).Handle(callerRequest(apiKeyCaller))

			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("accepts any scheme when none is named", func() {
			apiKeyCaller := &authkit.Principal{Subject: "svc_1", Scheme: authkit.SchemeAPIKey}

			res := guarded(nil).Handle(callerRequest(apiKeyCaller))

			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("the refusal itself", func() {
		It("uses the shared error shape", func() {
			res := guarded(nil).Handle(callerRequest(nil))

			envelope, ok := res.Body.(*restkit.ErrorEnvelope)
			Expect(ok).To(BeTrue())
			Expect(envelope.Error.Code).To(Equal(restkit.UnauthenticatedCode))
		})

		It("names authorization failures separately from authentication ones", func() {
			res := guarded(&restkit.SecuritySpec{Scopes: []string{"users:write"}}).
				Handle(callerRequest(jwtCaller()))

			envelope, ok := res.Body.(*restkit.ErrorEnvelope)
			Expect(ok).To(BeTrue())
			Expect(envelope.Error.Code).To(Equal(restkit.ForbiddenCode))
		})

		// Running validation first would tell an anonymous caller which fields
		// exist and how they are constrained.
		It("refuses before looking at the request body", func() {
			ep := &restkit.Endpoint[createReq, createRes]{
				BodyRules: []*rule.RuleSet{{Field: "name", Rules: []rule.Rule{vkit.Required()}}},
				Handle: func(r *rest.Request, body *createReq) (*rest.Result[createRes], error) {
					return rest.NewResult(&createRes{}), nil
				},
			}

			res := restkit.NewController(ep).Handle(jsonRequest(`{}`))

			Expect(res.Code).To(Equal(http.StatusUnauthorized),
				"an unauthenticated caller must not learn the validation rules")
		})
	})
})

var _ = Describe("RequiresAuthentication", func() {
	// An application whose endpoints need a caller but that configured no way to
	// verify one would refuse every request at runtime. Catching it at startup
	// turns that into an obvious misconfiguration.
	It("reports true when any endpoint needs a caller", func() {
		modules := []*rest.Module{{Path: "/things", Get: guarded(nil)}}

		Expect(restkit.RequiresAuthentication(modules)).To(BeTrue())
	})

	It("reports false when every endpoint is public", func() {
		modules := []*rest.Module{
			{Path: "/things", Get: guarded(&restkit.SecuritySpec{Public: true})},
			{Path: "/other", Post: guarded(&restkit.SecuritySpec{Public: true})},
		}

		Expect(restkit.RequiresAuthentication(modules)).To(BeFalse())
	})

	It("reports false for modules with no endpoints at all", func() {
		Expect(restkit.RequiresAuthentication(nil)).To(BeFalse())
	})
})
