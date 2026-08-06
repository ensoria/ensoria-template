package msgdoc_test

import (
	"encoding/json"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/validator/pkg/rule"
)

// userCreated stands in for a declared message payload type.
type userCreated struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// roundTrip encodes the spec and decodes it back, which is the trip the real
// spec makes between the describe program and encli.
func roundTrip(spec *msgdoc.MessagingSpec) *msgdoc.MessagingSpec {
	GinkgoHelper()
	raw, err := json.Marshal(spec)
	Expect(err).NotTo(HaveOccurred())
	var decoded msgdoc.MessagingSpec
	Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
	return &decoded
}

// rawJSON encodes the spec into a generic map, for asserting on which keys the
// document actually carries.
func rawJSON(v any) map[string]any {
	GinkgoHelper()
	raw, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	var out map[string]any
	Expect(json.Unmarshal(raw, &out)).To(Succeed())
	return out
}

var _ = Describe("MessagingSpec JSON round-trip", func() {
	It("preserves the whole spec across encode and decode", func() {
		idempotent := true
		spec := &msgdoc.MessagingSpec{
			Info:        &apidoc.Info{Title: "Ensoria", Version: "1.0.0"},
			Perspective: "ensoria-template",
			Servers: []*msgdoc.ServerSpec{{
				Name:        "rabbitmq-local",
				Protocol:    "amqp",
				Host:        "localhost:5672",
				Environment: "local",
			}},
			Operations: []*msgdoc.OperationSpec{{
				Action:   msgdoc.ActionReceive,
				Protocol: "rabbitmq",
				Channel: &msgdoc.ChannelSpec{
					Address:     "user.created",
					ServerNames: []string{"rabbitmq-local"},
				},
				Messages: []*msgdoc.MessageSpec{{
					Name:        "userCreated",
					ContentType: "application/json",
					Payload:     apidoc.BodySchema(reflect.TypeOf(userCreated{}), nil, apidoc.ExampleOptions{Resource: "user"}),
				}},
				Behavior: &msgdoc.Behavior{
					SideEffects: []string{"writes a row into users"},
					Idempotent:  &idempotent,
					Scopes:      []string{"users:write"},
					Ordering:    msgdoc.None,
					Delivery: &msgdoc.DeliverySpec{
						Guarantee: "at-least-once",
						Resolved: map[string]string{
							msgdoc.DeliveryErrorStrategy: "discard",
							msgdoc.DeliveryAutoAck:       "false",
						},
					},
				},
				Summary: "Consume the user-created event",
				Task:    "sync user",
			}},
			Conventions: &msgdoc.Conventions{
				Envelopes: []*msgdoc.EnvelopeSpec{{
					Protocol:     msgdoc.ProtocolWebSocket,
					TypeField:    "type",
					PayloadField: "data",
				}},
				DeliveryDefaults: map[string]string{msgdoc.DeliveryMaxRetries: "3"},
			},
		}

		decoded := roundTrip(spec)

		Expect(decoded.Perspective).To(Equal("ensoria-template"))
		Expect(decoded.Info.Title).To(Equal("Ensoria"))
		Expect(decoded.Servers).To(HaveLen(1))
		Expect(decoded.Servers[0].Host).To(Equal("localhost:5672"))

		Expect(decoded.Operations).To(HaveLen(1))
		op := decoded.Operations[0]
		Expect(op.Action).To(Equal(msgdoc.ActionReceive))
		Expect(op.Channel.Address).To(Equal("user.created"))
		Expect(op.Channel.ServerNames).To(Equal([]string{"rabbitmq-local"}))
		Expect(op.Behavior.Idempotent).NotTo(BeNil())
		Expect(*op.Behavior.Idempotent).To(BeTrue())
		Expect(op.Behavior.Delivery.Guarantee).To(Equal("at-least-once"))
		Expect(op.Behavior.Delivery.Resolved).To(HaveKeyWithValue(msgdoc.DeliveryErrorStrategy, "discard"))

		Expect(decoded.Conventions.Envelopes[0].TypeField).To(Equal("type"))
		Expect(decoded.Conventions.DeliveryDefaults).To(HaveKeyWithValue(msgdoc.DeliveryMaxRetries, "3"))
	})

	It("carries the payload schema tree, constraints and example through the trip", func() {
		payload := apidoc.BodySchema(
			reflect.TypeOf(userCreated{}),
			[]*rule.RuleSet{{Field: "name", Rules: []rule.Rule{vkit.Required(), vkit.MaxLength(10)}}},
			apidoc.ExampleOptions{Resource: "user"},
			map[string]string{"id": "identifier of the created user"},
		)
		spec := &msgdoc.MessagingSpec{Operations: []*msgdoc.OperationSpec{{
			Action:   msgdoc.ActionSend,
			Protocol: "rabbitmq",
			Channel:  &msgdoc.ChannelSpec{Address: "user.created"},
			Messages: []*msgdoc.MessageSpec{{Name: "userCreated", Payload: payload}},
		}}}

		decoded := roundTrip(spec)

		got := decoded.Operations[0].Messages[0].Payload
		Expect(got.Type).To(Equal(apidoc.TypeObject))
		Expect(got.Fields).To(HaveLen(3))
		Expect(got.Fields[0].Name).To(Equal("id"))
		Expect(got.Fields[0].Meaning).To(Equal("identifier of the created user"))
		Expect(got.Fields[1].Name).To(Equal("name"))
		Expect(got.Fields[1].Required).To(BeTrue())
		Expect(got.Fields[1].Schema.Constraints).To(ContainElement(
			HaveField("Code", "str_max_length"),
		))
		Expect(got.Example).NotTo(BeNil())
		// The example survives as decoded JSON, so a renderer can emit it as-is.
		Expect(got.Example).To(HaveKey("name"))
	})
})

var _ = Describe("undeclared versus explicitly none", func() {
	It("omits an undeclared behavior entirely", func() {
		op := &msgdoc.OperationSpec{
			Action:   msgdoc.ActionReceive,
			Protocol: "rabbitmq",
			Channel:  &msgdoc.ChannelSpec{Address: "orders"},
		}

		Expect(rawJSON(op)).NotTo(HaveKey("behavior"))
	})

	It("keeps an explicit none, so it is not mistaken for an omission", func() {
		op := &msgdoc.OperationSpec{
			Action:   msgdoc.ActionReceive,
			Protocol: "rabbitmq",
			Channel:  &msgdoc.ChannelSpec{Address: "orders"},
			Behavior: &msgdoc.Behavior{SideEffects: []string{msgdoc.None}},
		}

		behavior := rawJSON(op)["behavior"].(map[string]any)
		Expect(behavior["side_effects"]).To(Equal([]any{msgdoc.None}))
		// Nothing else was declared, so nothing else is claimed.
		Expect(behavior).NotTo(HaveKey("idempotent"))
		Expect(behavior).NotTo(HaveKey("ordering"))
	})

	It("distinguishes a declared false from an undeclared idempotency", func() {
		notIdempotent := false
		declared := rawJSON(&msgdoc.Behavior{Idempotent: &notIdempotent})
		undeclared := rawJSON(&msgdoc.Behavior{})

		// A declared false has to survive: it is the strongest warning the
		// document can carry, and omitempty on a bool would erase it.
		Expect(declared).To(HaveKeyWithValue("idempotent", false))
		Expect(undeclared).NotTo(HaveKey("idempotent"))
	})

	It("keeps operations as an explicit list even when there are none", func() {
		// Operations has no omitempty: an empty list says "we looked and found
		// nothing", where a missing key would look like a generator failure.
		Expect(rawJSON(&msgdoc.MessagingSpec{})).To(HaveKey("operations"))
	})
})
