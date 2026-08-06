package msgdoc_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
)

// op is a minimal operation, since these specs only care about the sort keys.
func op(address, action, protocol string) *msgdoc.OperationSpec {
	return &msgdoc.OperationSpec{
		Action:   action,
		Protocol: protocol,
		Channel:  &msgdoc.ChannelSpec{Address: address},
	}
}

// keys renders the sort keys of each operation, so a failure shows the order
// rather than a pile of pointers.
func keys(operations []*msgdoc.OperationSpec) []string {
	out := make([]string, 0, len(operations))
	for _, o := range operations {
		out = append(out, o.Channel.Address+" "+o.Action+" "+o.Protocol)
	}
	return out
}

var _ = Describe("SortOperations", func() {
	It("orders by channel address first", func() {
		operations := []*msgdoc.OperationSpec{
			op("zebra", msgdoc.ActionReceive, "rabbitmq"),
			op("apple", msgdoc.ActionReceive, "rabbitmq"),
		}

		msgdoc.SortOperations(operations)

		Expect(keys(operations)).To(Equal([]string{
			"apple receive rabbitmq",
			"zebra receive rabbitmq",
		}))
	})

	It("orders by action within one channel", func() {
		operations := []*msgdoc.OperationSpec{
			op("orders", msgdoc.ActionSend, "rabbitmq"),
			op("orders", msgdoc.ActionReceive, "rabbitmq"),
		}

		msgdoc.SortOperations(operations)

		// "receive" sorts before "send" alphabetically.
		Expect(keys(operations)).To(Equal([]string{
			"orders receive rabbitmq",
			"orders send rabbitmq",
		}))
	})

	It("breaks the remaining ties by protocol", func() {
		operations := []*msgdoc.OperationSpec{
			op("events", msgdoc.ActionSend, msgdoc.ProtocolWebSocket),
			op("events", msgdoc.ActionSend, "kafka"),
		}

		msgdoc.SortOperations(operations)

		Expect(keys(operations)).To(Equal([]string{
			"events send kafka",
			"events send websocket",
		}))
	})

	It("keeps the declared order of operations whose keys are equal", func() {
		first := op("orders", msgdoc.ActionReceive, "rabbitmq")
		first.Summary = "first"
		second := op("orders", msgdoc.ActionReceive, "rabbitmq")
		second.Summary = "second"
		operations := []*msgdoc.OperationSpec{first, second}

		msgdoc.SortOperations(operations)

		Expect(operations[0].Summary).To(Equal("first"))
		Expect(operations[1].Summary).To(Equal("second"))
	})

	It("does not panic on an operation without a channel", func() {
		operations := []*msgdoc.OperationSpec{
			op("orders", msgdoc.ActionReceive, "rabbitmq"),
			{Action: msgdoc.ActionSend},
		}

		Expect(func() { msgdoc.SortOperations(operations) }).NotTo(Panic())
		// The channel-less operation sorts first, on an empty address.
		Expect(operations[0].Channel).To(BeNil())
	})
})

var _ = Describe("SortServers", func() {
	It("orders servers by name", func() {
		servers := []*msgdoc.ServerSpec{
			{Name: "rabbitmq-local"},
			{Name: "kafka-local"},
		}

		msgdoc.SortServers(servers)

		Expect(servers[0].Name).To(Equal("kafka-local"))
		Expect(servers[1].Name).To(Equal("rabbitmq-local"))
	})
})

var _ = Describe("Build", func() {
	It("returns a spec whose operations are sorted", func() {
		spec := msgdoc.Build([]*msgdoc.OperationSpec{
			op("zebra", msgdoc.ActionReceive, "rabbitmq"),
			op("apple", msgdoc.ActionSend, "rabbitmq"),
		})

		Expect(keys(spec.Operations)).To(Equal([]string{
			"apple send rabbitmq",
			"zebra receive rabbitmq",
		}))
	})

	It("produces the same order regardless of how DI hands over the group", func() {
		declared := []*msgdoc.OperationSpec{
			op("apple", msgdoc.ActionReceive, "rabbitmq"),
			op("orders", msgdoc.ActionSend, "kafka"),
			op("/ws/user", msgdoc.ActionReceive, msgdoc.ProtocolWebSocket),
		}
		shuffled := []*msgdoc.OperationSpec{declared[2], declared[0], declared[1]}

		Expect(keys(msgdoc.Build(declared).Operations)).
			To(Equal(keys(msgdoc.Build(shuffled).Operations)))
	})
})
