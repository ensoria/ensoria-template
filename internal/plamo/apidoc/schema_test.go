package apidoc_test

import (
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
)

type address struct {
	City string `json:"city"`
	Zip  string `json:"zip,omitempty"`
}

type item struct {
	ID       string  `json:"id"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

type order struct {
	ID        string         `json:"id"`
	Note      *string        `json:"note"`
	Paid      bool           `json:"paid"`
	Tags      []string       `json:"tags"`
	Address   address        `json:"address"`
	Items     []item         `json:"items"`
	Labels    map[string]int `json:"labels"`
	CreatedAt time.Time      `json:"created_at"`
	ignored   string         //nolint:unused // unexported: must be skipped
}

// selfRef is a self-referencing type used to check that recursion terminates.
type selfRef struct {
	Name string   `json:"name"`
	Next *selfRef `json:"next"`
}

// noFields has no exported fields (restkit.NoBody equivalent) = no body.
type noFields struct{}

// directField looks up a field by name on one schema node (test helper).
func directField(s *apidoc.Schema, name string) *apidoc.Field {
	GinkgoHelper()
	Expect(s).NotTo(BeNil())
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	Fail("field not found: " + name)
	return nil
}

// fieldAt walks the schema tree with a dotted path ("items[].id") and returns
// the field it names. Kept independent of the production path resolver so the
// two cross-check each other.
func fieldAt(s *apidoc.Schema, path string) *apidoc.Field {
	GinkgoHelper()
	cur, segs := s, strings.Split(path, ".")
	for i, seg := range segs {
		name, arrays := seg, 0
		for strings.HasSuffix(name, "[]") {
			name = strings.TrimSuffix(name, "[]")
			arrays++
		}
		f := directField(cur, name)
		if i == len(segs)-1 {
			return f
		}
		cur = f.Schema
		for ; arrays > 0; arrays-- {
			Expect(cur.Items).NotTo(BeNil(), "expected an array at %q of %q", name, path)
			cur = cur.Items
		}
	}
	Fail("empty path")
	return nil
}

// schemaAt returns the schema node of the field named by path (test helper).
func schemaAt(s *apidoc.Schema, path string) *apidoc.Schema {
	GinkgoHelper()
	return fieldAt(s, path).Schema
}

// constraintByCode picks one constraint by code off a field's schema node (test helper).
func constraintByCode(f *apidoc.Field, code string) (apidoc.Constraint, bool) {
	return constraintByCode2(f.Schema.Constraints, code)
}

// constraintByCode2 picks one constraint by code out of a slice (test helper).
func constraintByCode2(cs []apidoc.Constraint, code string) (apidoc.Constraint, bool) {
	for _, c := range cs {
		if c.Code == code {
			return c, true
		}
	}
	return apidoc.Constraint{}, false
}

var _ = Describe("SchemaFromType", func() {
	var schema *apidoc.Schema

	BeforeEach(func() {
		schema = apidoc.SchemaFromType(reflect.TypeFor[order]())
	})

	It("uses json tag names and neutral scalar types", func() {
		Expect(schemaAt(schema, "id").Type).To(Equal(apidoc.TypeString))
		Expect(schemaAt(schema, "paid").Type).To(Equal(apidoc.TypeBoolean))
		Expect(schemaAt(schema, "items[].quantity").Type).To(Equal(apidoc.TypeInteger))
		Expect(schemaAt(schema, "items[].price").Type).To(Equal(apidoc.TypeNumber))
	})

	It("marks pointer fields nullable on the schema node", func() {
		Expect(schemaAt(schema, "note").Type).To(Equal(apidoc.TypeString))
		Expect(schemaAt(schema, "note").Nullable).To(BeTrue())
		Expect(schemaAt(schema, "id").Nullable).To(BeFalse())
	})

	It("marks omitempty fields as optional", func() {
		Expect(fieldAt(schema, "address.zip").Optional).To(BeTrue())
		Expect(fieldAt(schema, "address.city").Optional).To(BeFalse())
	})

	It("puts primitive slice elements under items", func() {
		tags := schemaAt(schema, "tags")

		Expect(tags.Type).To(Equal(apidoc.TypeArray))
		Expect(tags.Items.Type).To(Equal(apidoc.TypeString))
	})

	It("nests struct fields instead of flattening them", func() {
		addr := schemaAt(schema, "address")

		Expect(addr.Type).To(Equal(apidoc.TypeObject))
		Expect(addr.Fields).To(HaveLen(2))
		Expect(schemaAt(schema, "address.city").Type).To(Equal(apidoc.TypeString))
	})

	It("puts struct slice elements under items", func() {
		items := schemaAt(schema, "items")

		Expect(items.Type).To(Equal(apidoc.TypeArray))
		Expect(items.Items.Type).To(Equal(apidoc.TypeObject))
		Expect(schemaAt(schema, "items[].id").Type).To(Equal(apidoc.TypeString))
	})

	It("keeps the Go type name on object nodes so a renderer can name schemas", func() {
		Expect(schema.GoType).To(HaveSuffix("order"))
		Expect(schemaAt(schema, "items").Items.GoType).To(HaveSuffix("item"))
	})

	// Two packages can share a base name (several `dto` packages), so the type
	// name alone is not unique. The package path is what disambiguates them.
	It("keeps the package path alongside the type name", func() {
		Expect(schema.PkgPath).To(HaveSuffix("plamo/apidoc_test"))
		Expect(schemaAt(schema, "address").PkgPath).To(Equal(schema.PkgPath))
	})

	It("leaves the type name and package path empty for an anonymous struct", func() {
		s := apidoc.SchemaFromType(reflect.TypeFor[struct {
			Inline string `json:"inline"`
		}]())

		Expect(s.GoType).To(BeEmpty())
		Expect(s.PkgPath).To(BeEmpty())
	})

	It("records the value type of a dynamic-key object (map)", func() {
		labels := schemaAt(schema, "labels")

		Expect(labels.Type).To(Equal(apidoc.TypeObject))
		Expect(labels.Values).NotTo(BeNil())
		Expect(labels.Values.Type).To(Equal(apidoc.TypeInteger))
	})

	It("renders time.Time as a date-time string without recursing into it", func() {
		created := schemaAt(schema, "created_at")

		Expect(created.Type).To(Equal(apidoc.TypeString))
		Expect(created.Format).To(Equal(apidoc.FormatDateTime))
		Expect(created.Fields).To(BeEmpty())
	})

	It("skips unexported fields", func() {
		for _, f := range schema.Fields {
			Expect(f.Name).NotTo(Equal("ignored"))
		}
	})

	It("stops at a recursive type instead of descending forever", func() {
		s := apidoc.SchemaFromType(reflect.TypeFor[selfRef]())

		next := schemaAt(s, "next")
		Expect(next.Type).To(Equal(apidoc.TypeObject))
		Expect(next.GoType).To(HaveSuffix("selfRef"))
		Expect(next.Fields).To(BeEmpty())
	})

	It("expands a repeated type at every occurrence (only cycles are cut)", func() {
		type twoAddresses struct {
			Home address `json:"home"`
			Work address `json:"work"`
		}

		s := apidoc.SchemaFromType(reflect.TypeFor[twoAddresses]())

		Expect(schemaAt(s, "home").Fields).To(HaveLen(2))
		Expect(schemaAt(s, "work").Fields).To(HaveLen(2))
	})

	It("builds a schema for a non-struct body type", func() {
		s := apidoc.SchemaFromType(reflect.TypeFor[[]item]())

		Expect(s.Type).To(Equal(apidoc.TypeArray))
		Expect(s.Items.Type).To(Equal(apidoc.TypeObject))
		Expect(schemaAt(s.Items, "id").Type).To(Equal(apidoc.TypeString))
	})

	It("returns nil when there is no body", func() {
		Expect(apidoc.SchemaFromType(nil)).To(BeNil())
		Expect(apidoc.SchemaFromType(reflect.TypeFor[noFields]())).To(BeNil())
	})
})
