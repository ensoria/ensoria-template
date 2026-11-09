package describe

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/mb/pkg/mb"
)

// The kinds are written the way mb actually authenticates, so the document
// says what the running application does rather than what its settings look
// like.
var _ = Describe("brokerSecurity", func() {
	broker := func(t mb.BrokerType, url string, creds *mb.Credentials) *mb.Config {
		return &mb.Config{Type: t, URL: url, Credentials: creds}
	}
	kindsOf := func(security []*msgdoc.ServerSecurity) []string {
		var kinds []string
		for _, s := range security {
			kinds = append(kinds, s.Kind)
		}
		return kinds
	}

	DescribeTable("names the kind of credential the broker is reached with",
		func(config *mb.Config, kinds []string) {
			Expect(kindsOf(brokerSecurity(config))).To(Equal(kinds))
		},
		Entry("RabbitMQ with a user name", broker(mb.TypeRabbitMQ, "amqp://localhost:5672/",
			&mb.Credentials{Username: "u", Password: "p"}), []string{msgdoc.ServerAuthUserPassword}),
		Entry("RabbitMQ with the user in the URL", broker(mb.TypeRabbitMQ, "amqp://u:p@localhost:5672/", nil),
			[]string{msgdoc.ServerAuthUserPassword}),
		Entry("RabbitMQ with nothing", broker(mb.TypeRabbitMQ, "amqp://localhost:5672/", nil), nil),
		Entry("NATS with a user name and password", broker(mb.TypeNATS, "nats://localhost:4222",
			&mb.Credentials{Username: "u", Password: "p"}), []string{msgdoc.ServerAuthUserPassword}),
		Entry("NATS with a token", broker(mb.TypeNATS, "nats://localhost:4222",
			&mb.Credentials{Token: "t"}), []string{msgdoc.ServerAuthToken}),
		Entry("NATS with nothing", broker(mb.TypeNATS, "nats://localhost:4222", nil), nil),
		Entry("Kafka with no mechanism named, which mb sends as PLAIN", broker(mb.TypeKafka, "kafka:9092",
			&mb.Credentials{Username: "u", Password: "p"}), []string{msgdoc.ServerAuthPlain}),
		Entry("Kafka with SCRAM-SHA-256", broker(mb.TypeKafka, "kafka:9092",
			&mb.Credentials{Username: "u", Password: "p", SASLMechanism: "SCRAM-SHA-256"}), []string{msgdoc.ServerAuthScramSHA256}),
		Entry("Redpanda with SCRAM-SHA-512", broker(mb.TypeRedpanda, "redpanda:9092",
			&mb.Credentials{Username: "u", Password: "p", SASLMechanism: "SCRAM-SHA-512"}), []string{msgdoc.ServerAuthScramSHA512}),
		// mb turns SASL on only with both halves, so a mechanism alone
		// authenticates nothing.
		Entry("Kafka with a mechanism but no password", broker(mb.TypeKafka, "kafka:9092",
			&mb.Credentials{Username: "u", SASLMechanism: "PLAIN"}), nil),
		Entry("SQS, signed with AWS credentials", broker(mb.TypeSQS, "", nil), []string{msgdoc.ServerAuthAWSIAM}),
		Entry("Kinesis, signed with AWS credentials", broker(mb.TypeKinesis, "", nil), []string{msgdoc.ServerAuthAWSIAM}),
	)

	// A generated document is committed and shared.
	It("never carries the credential itself", func() {
		security := brokerSecurity(broker(mb.TypeRabbitMQ, "amqp://alice:hunter2@localhost:5672/",
			&mb.Credentials{Username: "alice", Password: "hunter2", Token: "tok"}))

		for _, s := range security {
			Expect(s.Description).NotTo(ContainSubstring("alice"))
			Expect(s.Description).NotTo(ContainSubstring("hunter2"))
			Expect(s.Description).NotTo(BeEmpty())
		}
	})

	It("is put on the broker's server", func() {
		server := brokerServer(broker(mb.TypeRabbitMQ, "amqp://localhost:5672/", &mb.Credentials{Username: "u", Password: "p"}), "local")

		Expect(kindsOf(server.Security)).To(Equal([]string{msgdoc.ServerAuthUserPassword}))
	})
})
