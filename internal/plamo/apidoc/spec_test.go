package apidoc_test

import (
	"encoding/json"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
)

// decodeJSON marshals v and decodes it back into a generic map (test helper).
func decodeJSON(v any) map[string]any {
	GinkgoHelper()
	b, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	var m map[string]any
	Expect(json.Unmarshal(b, &m)).To(Succeed())
	return m
}

// jsonFieldNamed picks one entry out of a JSON "fields" array by name (test helper).
func jsonFieldNamed(node map[string]any, name string) map[string]any {
	GinkgoHelper()
	fields, ok := node["fields"].([]any)
	Expect(ok).To(BeTrue(), "node has no fields array")
	for _, raw := range fields {
		f, ok := raw.(map[string]any)
		Expect(ok).To(BeTrue())
		if f["name"] == name {
			return f
		}
	}
	Fail("json field not found: " + name)
	return nil
}

// The JSON tags are the cross-repo contract: encli cannot import this internal
// package, so it mirrors these keys. Changing a key breaks the generator.
var _ = Describe("APISpec JSON contract", func() {
	It("emits the API info block", func() {
		spec := &apidoc.APISpec{
			Info: &apidoc.Info{Title: "Demo API", Version: "1.2.3", Description: "demo"},
		}

		info, ok := decodeJSON(spec)["info"].(map[string]any)

		Expect(ok).To(BeTrue())
		Expect(info["title"]).To(Equal("Demo API"))
		Expect(info["version"]).To(Equal("1.2.3"))
		Expect(info["description"]).To(Equal("demo"))
	})

	Describe("the schema tree", func() {
		var root map[string]any

		BeforeEach(func() {
			root = decodeJSON(apidoc.SchemaFromType(reflect.TypeFor[order]()))
		})

		It("emits an object node with its Go type name and fields", func() {
			Expect(root["type"]).To(Equal("object"))
			Expect(root["go_type"]).To(HaveSuffix("order"))
			Expect(root["fields"]).NotTo(BeEmpty())
		})

		It("emits each field with its own nested schema node", func() {
			id := jsonFieldNamed(root, "id")

			schema, ok := id["schema"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(schema["type"]).To(Equal("string"))
		})

		It("emits array elements under items", func() {
			items := jsonFieldNamed(root, "items")["schema"].(map[string]any)

			Expect(items["type"]).To(Equal("array"))
			Expect(items["items"].(map[string]any)["type"]).To(Equal("object"))
		})

		It("emits map value types under values", func() {
			labels := jsonFieldNamed(root, "labels")["schema"].(map[string]any)

			Expect(labels["type"]).To(Equal("object"))
			Expect(labels["values"].(map[string]any)["type"]).To(Equal("integer"))
		})

		It("emits nullable and format on the schema node", func() {
			note := jsonFieldNamed(root, "note")["schema"].(map[string]any)
			created := jsonFieldNamed(root, "created_at")["schema"].(map[string]any)

			Expect(note["nullable"]).To(BeTrue())
			Expect(created["format"]).To(Equal("date-time"))
		})

		It("emits optional on the field, not on the schema node", func() {
			addr := jsonFieldNamed(root, "address")["schema"].(map[string]any)

			Expect(jsonFieldNamed(addr, "zip")["optional"]).To(BeTrue())
		})
	})
})
