package apidoc_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/rest/pkg/rest"
)

// sameShape appears as both the request and the response of the endpoint below,
// which is what makes one set of field docs ambiguous.
type sameShape struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

var _ = Describe("field documentation", func() {
	describeWith := func(ep *restkit.Endpoint[sameShape, sameShape]) *apidoc.EndpointSpec {
		ep.Success = http.StatusOK
		ep.Security = &restkit.SecuritySpec{Public: true}
		ep.Handle = func(r *rest.Request, req *sameShape) (*rest.Result[sameShape], error) {
			return rest.NewResult(req), nil
		}
		module := &rest.Module{Path: "/things", Post: restkit.NewController(ep)}
		return apidoc.DescribeModule(module, nil)[0]
	}

	meaning := func(schema *apidoc.Schema, name string) string {
		GinkgoHelper()
		for _, f := range schema.Fields {
			if f.Name == name {
				return f.Meaning
			}
		}
		Fail("no such field: " + name)
		return ""
	}

	It("applies a shared declaration to both sides", func() {
		spec := describeWith(&restkit.Endpoint[sameShape, sameShape]{
			FieldDocs: map[string]string{"name": "User display name"},
		})

		Expect(meaning(spec.Request, "name")).To(Equal("User display name"))
		Expect(meaning(spec.Response, "name")).To(Equal("User display name"))
	})

	// The same field name exists on both sides, so wording that only makes
	// sense for one of them has to be declared for that side alone.
	It("applies a request declaration to the request only", func() {
		spec := describeWith(&restkit.Endpoint[sameShape, sameShape]{
			RequestFieldDocs: map[string]string{"name": "Omit to leave it unchanged"},
		})

		Expect(meaning(spec.Request, "name")).To(Equal("Omit to leave it unchanged"))
		Expect(meaning(spec.Response, "name")).To(BeEmpty())
	})

	It("applies a response declaration to the response only", func() {
		spec := describeWith(&restkit.Endpoint[sameShape, sameShape]{
			ResponseFieldDocs: map[string]string{"name": "The name as stored"},
		})

		Expect(meaning(spec.Request, "name")).To(BeEmpty())
		Expect(meaning(spec.Response, "name")).To(Equal("The name as stored"))
	})

	// A declaration limited to one side is the more specific statement, so it
	// wins over the shared one.
	It("lets a side declaration win over the shared one", func() {
		spec := describeWith(&restkit.Endpoint[sameShape, sameShape]{
			FieldDocs:        map[string]string{"name": "User display name"},
			RequestFieldDocs: map[string]string{"name": "Omit to leave it unchanged"},
		})

		Expect(meaning(spec.Request, "name")).To(Equal("Omit to leave it unchanged"))
		Expect(meaning(spec.Response, "name")).To(Equal("User display name"))
	})

	It("leaves fields the side declaration does not mention on the shared text", func() {
		spec := describeWith(&restkit.Endpoint[sameShape, sameShape]{
			FieldDocs:        map[string]string{"name": "User display name", "note": "Free text"},
			RequestFieldDocs: map[string]string{"name": "Omit to leave it unchanged"},
		})

		Expect(meaning(spec.Request, "note")).To(Equal("Free text"))
		Expect(meaning(spec.Response, "note")).To(Equal("Free text"))
	})
})

// idHolder has an id field, so the example generator has something to prefix.
type idHolder struct {
	ID string `json:"id"`
}

var _ = Describe("resource naming for internal endpoints", func() {
	readOnly := func() rest.Controller {
		return restkit.NewController(&restkit.Endpoint[restkit.NoBody, idHolder]{
			Success:  http.StatusOK,
			Security: &restkit.SecuritySpec{Public: true},
			Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[idHolder], error) {
				return rest.NewResult(&idHolder{}), nil
			},
		})
	}

	withPrefix := func(prefix string) rest.Controller {
		return restkit.NewController(&restkit.Endpoint[restkit.NoBody, idHolder]{
			Success:  http.StatusOK,
			Security: &restkit.SecuritySpec{Public: true},
			IDPrefix: prefix,
			Handle: func(r *rest.Request, _ *restkit.NoBody) (*rest.Result[idHolder], error) {
				return rest.NewResult(&idHolder{}), nil
			},
		})
	}

	exampleID := func(spec *apidoc.EndpointSpec) string {
		GinkgoHelper()
		example, ok := spec.Response.Example.(map[string]any)
		Expect(ok).To(BeTrue())
		id, ok := example["id"].(string)
		Expect(ok).To(BeTrue())
		return id
	}

	// An application's own endpoints live under a marker segment, and the
	// segment after it is not a resource name. Reading it as one would make
	// /_/tasks share a project's /tasks declarations.
	It("keeps an internal endpoint apart from a project resource of the same name", func() {
		spec := apidoc.Build([]*rest.Module{
			{Path: "/tasks", Get: withPrefix("tsk")},
			{Path: "/_/tasks", Get: readOnly()},
		})

		var project, internal *apidoc.EndpointSpec
		for _, s := range spec.Endpoints {
			if s.Path == "/tasks" {
				project = s
			} else {
				internal = s
			}
		}

		Expect(exampleID(project)).To(HavePrefix("tsk_"))
		Expect(exampleID(internal)).NotTo(HavePrefix("tsk_"))
	})
})
