package apidoc_test

import (
	"reflect"
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
	ID        string    `json:"id"`
	Note      *string   `json:"note"`
	Paid      bool      `json:"paid"`
	Tags      []string  `json:"tags"`
	Address   address   `json:"address"`
	Items     []item    `json:"items"`
	CreatedAt time.Time `json:"created_at"`
	ignored   string    //nolint:unused // unexported: must be skipped
}

// fieldByName は Fields から名前で1件取り出す(テスト補助)。
func fieldByName(s *apidoc.Schema, name string) apidoc.Field {
	GinkgoHelper()
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	Fail("field not found: " + name)
	return apidoc.Field{}
}

var _ = Describe("SchemaFromType", func() {
	var schema *apidoc.Schema

	BeforeEach(func() {
		schema = apidoc.SchemaFromType(reflect.TypeFor[order]())
	})

	It("uses json tag names and normalized primitive types", func() {
		Expect(fieldByName(schema, "id").Type).To(Equal("string"))
		Expect(fieldByName(schema, "paid").Type).To(Equal("bool"))
	})

	It("marks pointer fields as nullable", func() {
		Expect(fieldByName(schema, "note").Nullable).To(BeTrue())
		Expect(fieldByName(schema, "id").Nullable).To(BeFalse())
	})

	It("marks omitempty fields as optional", func() {
		Expect(fieldByName(schema, "address.zip").Optional).To(BeTrue())
		Expect(fieldByName(schema, "address.city").Optional).To(BeFalse())
	})

	It("renders primitive slices as T[]", func() {
		Expect(fieldByName(schema, "tags").Type).To(Equal("string[]"))
	})

	It("flattens nested structs with dot notation", func() {
		Expect(fieldByName(schema, "address").Type).To(Equal("object"))
		Expect(fieldByName(schema, "address.city").Type).To(Equal("string"))
	})

	It("flattens struct slices with [] notation", func() {
		Expect(fieldByName(schema, "items").Type).To(Equal("object[]"))
		Expect(fieldByName(schema, "items[].id").Type).To(Equal("string"))
		Expect(fieldByName(schema, "items[].quantity").Type).To(Equal("int"))
		Expect(fieldByName(schema, "items[].price").Type).To(Equal("float"))
	})

	It("renders time.Time as an RFC 3339 string without recursing", func() {
		Expect(fieldByName(schema, "created_at").Type).To(Equal("string (RFC 3339)"))
	})

	It("skips unexported fields", func() {
		for _, f := range schema.Fields {
			Expect(f.Name).NotTo(Equal("ignored"))
		}
	})

	It("returns nil for non-struct types", func() {
		Expect(apidoc.SchemaFromType(reflect.TypeFor[string]())).To(BeNil())
		Expect(apidoc.SchemaFromType(nil)).To(BeNil())
	})
})
