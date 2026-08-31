package mb_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/infra/mb"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	enmb "github.com/ensoria/mb/pkg/mb"
	"go.uber.org/fx"
)

const (
	// testEnv is any valid environment name: the specs that load a configuration
	// load an empty one, so which environment it claims to be does not matter.
	testEnv = "test"

	brokerURL       = "amqp://broker.internal:5672/"
	brokerQueueName = "orders"
	brokerUser      = "myuser"
	brokerPassword  = "mypassword"
	brokerToken     = "mytoken"
	saslMechanism   = "SCRAM-SHA-512"
)

// fakeLifecycle stands in for fx's lifecycle so that a constructor can be
// called outside an application.
type fakeLifecycle struct {
	hooks []fx.Hook
}

func (l *fakeLifecycle) Append(h fx.Hook) { l.hooks = append(l.hooks, h) }

// paramsWith builds the resolved parameters the assembly step reads.
func paramsWith(broker *appconfig.Broker) *appconfig.Parameters {
	return &appconfig.Parameters{Broker: broker}
}

var _ = Describe("BrokerConfig", func() {
	// The registry these specs read is their own, built here and seen by
	// nothing else. What they need — a registry that is loaded but holds no
	// default module, so that reading "default" fails with an error rather than
	// raising the panic an untouched registry answers with — is therefore just
	// a value they construct, not a process-wide state they take a turn at.
	Context("when the configuration cannot be read", func() {
		var reg *registry.Registry

		BeforeEach(func() {
			reg = registry.New()
			Expect(reg.LoadConfigurationFiles(os.DirFS("."), testEnv, map[string]string{}, true)).To(Succeed())
		})

		It("reports the failure instead of passing for an unconfigured broker", func() {
			config, err := mb.BrokerConfig(reg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broker configuration unavailable"))
			Expect(config).To(BeNil())
		})

		It("stops the subscriber from being built", func() {
			_, err := mb.NewSubscriberConnection(nil)(&fakeLifecycle{}, reg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broker configuration unavailable"))
		})

		It("stops the publisher from being built", func() {
			_, err := mb.NewPublisherConnection(nil)(&fakeLifecycle{}, reg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broker configuration unavailable"))
		})
	})
})

var _ = Describe("brokerConfigFromParams", func() {
	Context("when no broker is configured", func() {
		It("yields nil for an empty broker section", func() {
			Expect(mb.BrokerConfigFromParams(paramsWith(&appconfig.Broker{}))).To(BeNil())
		})

		It("yields nil when the section is absent altogether", func() {
			Expect(mb.BrokerConfigFromParams(paramsWith(nil))).To(BeNil())
		})
	})

	Context("when a broker is configured", func() {
		It("carries the connection over", func() {
			config := mb.BrokerConfigFromParams(paramsWith(&appconfig.Broker{
				Type:      appconfig.BrokerTypeRabbitMQ,
				URL:       brokerURL,
				QueueName: brokerQueueName,
			}))

			Expect(config).NotTo(BeNil())
			Expect(config.Type).To(Equal(enmb.BrokerType(appconfig.BrokerTypeRabbitMQ)))
			Expect(config.URL).To(Equal(brokerURL))
			Expect(config.QueueName).To(Equal(brokerQueueName))
		})

		It("leaves the credentials off when none are set", func() {
			config := mb.BrokerConfigFromParams(paramsWith(&appconfig.Broker{
				Type: appconfig.BrokerTypeNATS,
				URL:  brokerURL,
			}))

			Expect(config).NotTo(BeNil())
			Expect(config.Credentials).To(BeNil())
		})

		It("carries a username and password over", func() {
			config := mb.BrokerConfigFromParams(paramsWith(&appconfig.Broker{
				Type:     appconfig.BrokerTypeRabbitMQ,
				URL:      brokerURL,
				Username: brokerUser,
				Password: brokerPassword,
			}))

			Expect(config.Credentials).NotTo(BeNil())
			Expect(config.Credentials.Username).To(Equal(brokerUser))
			Expect(config.Credentials.Password).To(Equal(brokerPassword))
		})

		It("carries a token over", func() {
			config := mb.BrokerConfigFromParams(paramsWith(&appconfig.Broker{
				Type:  appconfig.BrokerTypeNATS,
				URL:   brokerURL,
				Token: brokerToken,
			}))

			Expect(config.Credentials).NotTo(BeNil())
			Expect(config.Credentials.Token).To(Equal(brokerToken))
		})

		It("carries a SASL mechanism over even without a username", func() {
			config := mb.BrokerConfigFromParams(paramsWith(&appconfig.Broker{
				Type:          appconfig.BrokerTypeKafka,
				URL:           brokerURL,
				SASLMechanism: saslMechanism,
			}))

			Expect(config.Credentials).NotTo(BeNil())
			Expect(config.Credentials.SASLMechanism).To(Equal(saslMechanism))
		})
	})
})

// dikit.LC is what the connection constructors take; the fake has to satisfy it.
var _ dikit.LC = (*fakeLifecycle)(nil)
