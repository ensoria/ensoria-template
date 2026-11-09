package describe

import (
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authApp "github.com/ensoria/ensoria-template/internal/app/auth"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
)

var _ = Describe("initiatorHeaderSchema", func() {
	fieldNamed := func(schema *apidoc.Schema, name string) *apidoc.Field {
		GinkgoHelper()
		for _, f := range schema.Fields {
			if f.Name == name {
				return f
			}
		}
		Fail("no field " + name)
		return nil
	}

	It("describes the three headers by the names initiator writes them under", func() {
		schema := initiatorHeaderSchema(nil)

		var names []string
		for _, f := range schema.Fields {
			names = append(names, f.Name)
		}
		Expect(names).To(Equal([]string{initiator.HeaderKind, initiator.HeaderID, initiator.HeaderClaims}))
	})

	// In AsyncAPI one message definition serves both the send and the receive
	// side of a channel, and other producers need not write them at all.
	It("requires none of them", func() {
		for _, f := range initiatorHeaderSchema(nil).Fields {
			Expect(f.Required).To(BeFalse(), f.Name)
			Expect(f.Meaning).NotTo(BeEmpty(), f.Name)
		}
	})

	It("lists every kind the kind header can carry", func() {
		kind := fieldNamed(initiatorHeaderSchema(nil), initiator.HeaderKind)

		var kinds []any
		for _, k := range initiator.Kinds() {
			kinds = append(kinds, string(k))
		}
		Expect(kind.Schema.Constraints).To(ContainElement(apidoc.Constraint{
			Code: "str_any_of", Params: map[string]any{"values": kinds},
		}))
	})

	// Received claims are filtered by this application's own list, so the
	// list is exact in both directions — and it is the only thing a producer
	// can learn it from.
	It("names the claims carried, and says any other is dropped on receipt", func() {
		claims := fieldNamed(initiatorHeaderSchema([]string{"tenant_id", "org"}), initiator.HeaderClaims)

		Expect(claims.Meaning).To(ContainSubstring("tenant_id, org"))
		Expect(claims.Meaning).To(ContainSubstring("dropped"))
	})

	It("says so when no claim is carried at all", func() {
		claims := fieldNamed(initiatorHeaderSchema(nil), initiator.HeaderClaims)

		Expect(claims.Meaning).To(ContainSubstring("none"))
	})

	It("names the Go fields after the headers they stand for", func() {
		t := reflect.TypeFor[InitiatorHeaders]()
		for name, header := range map[string]string{"Kind": initiator.HeaderKind, "ID": initiator.HeaderID, "Claims": initiator.HeaderClaims} {
			field, ok := t.FieldByName(name)
			Expect(ok).To(BeTrue(), name)
			Expect(field.Tag.Get("json")).To(Equal(header+",omitempty"), name)
		}
	})
})

var _ = Describe("websocketConvention", func() {
	convention := websocketConvention(90 * time.Second)

	It("explains how a connection is authenticated", func() {
		Expect(convention.Authentication).NotTo(BeEmpty())
	})

	It("describes the error message with every code it can carry", func() {
		Expect(convention.Error.MessageType).To(Equal(wskit.ErrorMessageName))
		Expect(convention.Error.Payload).NotTo(BeNil())

		var codes []string
		for _, c := range convention.Error.Codes {
			codes = append(codes, c.Code)
		}
		var declared []string
		for _, c := range wskit.ErrorCodes() {
			declared = append(declared, c.Code)
		}
		Expect(codes).To(Equal(declared))
	})

	It("says how often a connection is asked about again, and how one is ended", func() {
		Expect(convention.Recheck.Interval).To(Equal("1m30s"))
		Expect(convention.Recheck.Description).NotTo(BeEmpty())
		Expect(convention.Recheck.CloseCodes).To(HaveLen(len(wskit.CloseCodes())))
		for i, c := range wskit.CloseCodes() {
			Expect(convention.Recheck.CloseCodes[i]).To(Equal(&msgdoc.CloseCodeMeaning{
				Code: int(c.Code), Meaning: c.Meaning, ClientAction: c.ClientAction,
			}))
		}
	})
})

// Built without a configuration, which this package does not initialize: what
// is asserted here does not depend on one.
var _ = Describe("buildMessagingConventions", func() {
	conv := buildMessagingConventions(nil)

	// Taken from the chain the router builds rather than restated, so the two
	// cannot say different things.
	It("lists what runs on every handshake, from the router itself", func() {
		Expect(conv.GlobalMiddlewares).To(Equal(wsApp.UpgradeGuardNames()))
	})

	It("carries the application's scope policy", func() {
		Expect(conv.ScopeImplications).To(Equal(authApp.ScopeImplicationTable()))
	})

	It("explains the headers that say who started the work", func() {
		Expect(conv.Initiator.Headers).To(Equal([]string{initiator.HeaderKind, initiator.HeaderID, initiator.HeaderClaims}))
		Expect(conv.Initiator.Description).To(ContainSubstring("not a credential"))
	})

	It("states the WebSocket conventions", func() {
		Expect(conv.WebSocket).NotTo(BeNil())
	})
})
