package wskit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
)

var _ = Describe("MessageTypeOf", func() {
	It("reads the type from an envelope, leaving the payload alone", func() {
		Expect(wskit.MessageTypeOf([]byte(`{"type":"user.echo","data":{"message":"hi"}}`))).To(Equal("user.echo"))
	})

	It("reads it from an envelope that carries no payload", func() {
		Expect(wskit.MessageTypeOf([]byte(`{"type":"user.whoami"}`))).To(Equal("user.whoami"))
	})

	DescribeTable("is empty for a frame that names no type",
		func(frame string) {
			Expect(wskit.MessageTypeOf([]byte(frame))).To(BeEmpty())
		},
		Entry("not JSON", `not json at all`),
		Entry("no type", `{"data":{"message":"hi"}}`),
		Entry("a type that is not a string", `{"type":42}`),
	)
})

var _ = Describe("ErrorCodes", func() {
	// The generated document lists them, so a code a client can receive and
	// the document does not explain is a client left guessing.
	It("explains every code the error message can carry, once each", func() {
		var codes []string
		for _, c := range wskit.ErrorCodes() {
			Expect(c.Meaning).NotTo(BeEmpty(), c.Code)
			codes = append(codes, c.Code)
		}

		Expect(codes).To(ConsistOf(
			wskit.CodeNotParsable,
			wskit.CodeValidationFailed,
			wskit.CodeUnknownMessageType,
			wskit.CodeUnauthenticated,
			wskit.CodeForbidden,
		))
	})
})
