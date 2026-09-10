package restkit_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// order is the resource the specs authorize against. It stands in for a domain
// object, which is what a real declaration names.
type order struct {
	OwnerSubject string
}

// otherResource exists only so that a handler can hand Authorize the wrong
// type, which is one of the two ways a declaration and a handler can disagree.
type otherResource struct{}

// ownerCheck is the declaration under test: the constraint in prose, and the
// predicate that decides it.
func ownerCheck() *restkit.ResourceCheck {
	return restkit.NewResourceCheck[order](
		"Only the owner of the order can update it.",
		func(p *authkit.Principal, o *order) bool { return o.OwnerSubject == p.Subject },
	)
}

// authorizing builds a controller whose handler runs handle, under the given
// security declaration. handle receives the request so that it can call (or
// deliberately not call) restkit.Authorize.
func authorizing(
	security *restkit.SecuritySpec, handle func(r *rest.Request) error,
) rest.Controller {
	return restkit.NewController(&restkit.Endpoint[restkit.NoBody, okBody]{
		Security: security,
		Success:  http.StatusOK,
		Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[okBody], error) {
			if err := handle(r); err != nil {
				return nil, err
			}
			return rest.NewResult(&okBody{OK: true}), nil
		},
	})
}

// ownerRequest builds a request from a caller with the given subject.
func ownerRequest(subject string) *rest.Request {
	r := rest.NewRequest(httptest.NewRequest(http.MethodPut, "/order/1", nil))
	r.SetContext(authkit.WithPrincipal(r.Context(), &authkit.Principal{
		Subject: subject, Scheme: authkit.SchemeJWT,
	}))
	return r
}

// driftRecords keeps the records written under the authorization drift type,
// which is what an alert would match on.
func driftRecords(records []map[string]any) []map[string]any {
	var drift []map[string]any
	for _, record := range records {
		if record["type"] == restkit.LogTypeAuthorizationDrift {
			drift = append(drift, record)
		}
	}
	return drift
}

var _ = Describe("resource authorization", func() {
	declared := &restkit.SecuritySpec{Resource: ownerCheck()}

	Describe("a handler that authorizes the resource", func() {
		It("serves the owner", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return restkit.Authorize(r, &order{OwnerSubject: "usr_1"})
			})

			res := ctrl.Handle(ownerRequest("usr_1"))

			Expect(res.Code).To(Equal(http.StatusOK))
		})

		// The refusal is the same 403 a missing scope produces. A caller cannot
		// act on the difference — neither is something presenting the
		// credential again would fix — so they are not told there is one.
		It("refuses everybody else with the shared 403", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return restkit.Authorize(r, &order{OwnerSubject: "usr_2"})
			})

			res := ctrl.Handle(ownerRequest("usr_1"))

			Expect(res.Code).To(Equal(http.StatusForbidden))
			envelope, ok := res.Body.(*restkit.ErrorEnvelope)
			Expect(ok).To(BeTrue())
			Expect(envelope.Error.Code).To(Equal(restkit.ForbiddenCode))
		})

		It("writes no record when nothing is wrong", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return restkit.Authorize(r, &order{OwnerSubject: "usr_1"})
			})

			records := captureLogRecords(func() { ctrl.Handle(ownerRequest("usr_1")) })

			Expect(driftRecords(records)).To(BeEmpty())
		})

		// A handler loading several rows runs the same rule on each of them.
		It("can be called more than once", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return errors.Join(
					restkit.Authorize(r, &order{OwnerSubject: "usr_1"}),
					restkit.Authorize(r, &order{OwnerSubject: "usr_1"}),
				)
			})

			Expect(ctrl.Handle(ownerRequest("usr_1")).Code).To(Equal(http.StatusOK))
		})
	})

	// Only reachable by declaring Public alongside a resource check — which the
	// startup checks refuse — or by calling a handler directly. Refusing is the
	// honest answer either way, and it keeps a nil caller out of the predicate.
	Describe("a request carrying no caller", func() {
		It("is refused rather than handed to the predicate", func() {
			ctrl := authorizing(&restkit.SecuritySpec{Public: true, Resource: ownerCheck()},
				func(r *rest.Request) error {
					return restkit.Authorize(r, &order{OwnerSubject: "usr_1"})
				})
			anonymous := rest.NewRequest(httptest.NewRequest(http.MethodPut, "/order/1", nil))

			res := ctrl.Handle(anonymous)

			Expect(res.Code).To(Equal(http.StatusForbidden))
		})
	})

	// The defect the whole mechanism exists to catch: the declaration promises
	// a constraint, and the handler serves everyone.
	Describe("a handler that forgets to authorize", func() {
		forgetful := func() rest.Controller {
			return authorizing(declared, func(*rest.Request) error { return nil })
		}

		It("does not serve the success it was about to", func() {
			res := forgetful().Handle(ownerRequest("usr_1"))

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})

		It("says which endpoint, under a type an alert can match", func() {
			records := driftRecords(captureLogRecords(func() {
				forgetful().Handle(ownerRequest("usr_1"))
			}))

			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("method", http.MethodPut))
			Expect(records[0]).To(HaveKeyWithValue("path", "/order/1"))
			Expect(records[0]["msg"]).To(ContainSubstring("never ran"))
		})

		// The check is not a development aid. Outside a developer's machine the
		// same forgotten call is the same resource served to whoever asked, so
		// the strict-mode flag has no say here.
		It("is caught whatever the declaration mode says", func() {
			restkit.SetStrictDeclarations(false)
			defer restkit.SetStrictDeclarations(restkit.InitialStrictDeclarations())

			res := forgetful().Handle(ownerRequest("usr_1"))

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})

		// A handler that refuses the request never served the resource, so
		// there was nothing to authorize.
		It("is not reported when the handler answered with an error", func() {
			ctrl := authorizing(declared, func(*rest.Request) error {
				return restkit.NewError(http.StatusNotFound, "order_not_found", "no such order")
			})

			var res *rest.Response
			records := captureLogRecords(func() { res = ctrl.Handle(ownerRequest("usr_1")) })

			Expect(res.Code).To(Equal(http.StatusNotFound))
			Expect(driftRecords(records)).To(BeEmpty())
		})
	})

	// The mirror defect: the handler is stricter than the document. Less
	// dangerous, and still refused — there is no answer Authorize could give
	// instead, because "allowed" would let the handler serve a resource it
	// believes was checked.
	Describe("a handler that authorizes without a declaration", func() {
		undeclared := func() rest.Controller {
			return authorizing(nil, func(r *rest.Request) error {
				return restkit.Authorize(r, &order{OwnerSubject: "usr_1"})
			})
		}

		It("answers 500 rather than serving the resource", func() {
			res := undeclared().Handle(ownerRequest("usr_1"))

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})

		It("says what the handler passed and what to declare", func() {
			records := driftRecords(captureLogRecords(func() {
				undeclared().Handle(ownerRequest("usr_1"))
			}))

			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("passed_resource", "*restkit_test.order"))
			Expect(records[0]["msg"]).To(ContainSubstring("does not declare"))
		})

		// One defect, one record. The handler ignoring the error it was given
		// must not turn into a second report of the same thing.
		It("writes one record even when the handler ignores the error", func() {
			ctrl := authorizing(nil, func(r *rest.Request) error {
				_ = restkit.Authorize(r, &order{OwnerSubject: "usr_1"})
				return nil
			})

			var res *rest.Response
			records := captureLogRecords(func() { res = ctrl.Handle(ownerRequest("usr_1")) })

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
			Expect(driftRecords(records)).To(HaveLen(1))
		})
	})

	// A failed type assertion inside the predicate would be a panic with a
	// stack of generic frames. Comparing the types first makes it a named
	// defect that says both halves.
	Describe("a handler that passes the wrong type", func() {
		It("answers 500 and names both types", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return restkit.Authorize(r, &otherResource{})
			})

			var res *rest.Response
			records := captureLogRecords(func() { res = ctrl.Handle(ownerRequest("usr_1")) })

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
			drift := driftRecords(records)
			Expect(drift).To(HaveLen(1))
			Expect(drift[0]).To(HaveKeyWithValue("declared_resource", "*restkit_test.order"))
			Expect(drift[0]).To(HaveKeyWithValue("passed_resource", "*restkit_test.otherResource"))
		})

		// A value where a pointer was declared is the mistake most likely to be
		// made, and it has to be caught rather than silently passed on.
		It("counts a value where a pointer was declared", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return restkit.Authorize(r, order{OwnerSubject: "usr_1"})
			})

			Expect(ctrl.Handle(ownerRequest("usr_1")).Code).To(Equal(http.StatusInternalServerError))
		})

		It("counts nothing at all", func() {
			ctrl := authorizing(declared, func(r *rest.Request) error {
				return restkit.Authorize(r, nil)
			})

			var res *rest.Response
			records := captureLogRecords(func() { res = ctrl.Handle(ownerRequest("usr_1")) })

			Expect(res.Code).To(Equal(http.StatusInternalServerError))
			Expect(driftRecords(records)[0]).To(HaveKeyWithValue("passed_resource", "nil"))
		})
	})

	// The check exists only where it is declared. An endpoint that never
	// mentions a resource must be served exactly as it was before any of this.
	Describe("an endpoint that declares no resource check", func() {
		It("serves a handler that never calls Authorize", func() {
			ctrl := authorizing(&restkit.SecuritySpec{Scopes: []string{"orders:write"}},
				func(*rest.Request) error { return nil })

			r := ownerRequest("usr_1")
			r.SetContext(authkit.WithPrincipal(r.Context(), &authkit.Principal{
				Subject: "usr_1", Scheme: authkit.SchemeJWT, Scopes: []string{"orders:write"},
			}))

			var res *rest.Response
			records := captureLogRecords(func() { res = ctrl.Handle(r) })

			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(driftRecords(records)).To(BeEmpty())
		})
	})

	// Called from a raw rest.Controller, or from a handler invoked by something
	// other than the adapter. Nothing else can report it, so Authorize does.
	Describe("Authorize called outside a typed endpoint", func() {
		It("refuses and reports rather than answering allowed", func() {
			r := ownerRequest("usr_1")

			var err error
			records := captureLogRecords(func() { err = restkit.Authorize(r, &order{}) })

			Expect(err).To(HaveOccurred())
			Expect(driftRecords(records)).To(HaveLen(1))
		})
	})

	Describe("PublicResourceChecks", func() {
		// A resource rule decides whether this caller may touch this resource,
		// and a public endpoint has no caller. Worse, authorize() lets a public
		// request past before any check runs, so the endpoint would serve
		// everyone while its document described a constraint.
		It("names a public endpoint that declares one", func() {
			modules := []*rest.Module{{
				Path: "/order/{id}",
				Put:  authorizing(&restkit.SecuritySpec{Public: true, Resource: ownerCheck()}, nil),
			}}

			Expect(restkit.PublicResourceChecks(modules)).To(Equal([]string{"PUT /order/{id}"}))
		})

		It("says nothing about a resource check on an endpoint that needs a caller", func() {
			modules := []*rest.Module{{Path: "/order/{id}", Put: authorizing(declared, nil)}}

			Expect(restkit.PublicResourceChecks(modules)).To(BeEmpty())
		})

		It("says nothing about a public endpoint without one", func() {
			modules := []*rest.Module{{
				Path: "/health",
				Get:  authorizing(&restkit.SecuritySpec{Public: true}, nil),
			}}

			Expect(restkit.PublicResourceChecks(modules)).To(BeEmpty())
		})
	})
})
