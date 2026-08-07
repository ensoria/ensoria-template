package describe

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	"github.com/ensoria/ensoria-template/internal/app/apiinfo"
	httpdto "github.com/ensoria/ensoria-template/internal/app/http/dto"
	inframb "github.com/ensoria/ensoria-template/internal/infra/mb"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/worker/pkg/worker"
	"go.uber.org/fx"
)

// Perspective names the application whose point of view every operation is
// written from. "send" means this application sends.
const Perspective = "ensoria-template"

// Server names used in the generated document.
const (
	wsServerName     = "websocket"
	brokerServerName = "broker"
)

// BuildMessaging resolves the messaging declarations without real infrastructure
// and returns the MessagingSpec.
func BuildMessaging(envVal *string) (*msgdoc.MessagingSpec, error) {
	registry.InitializeConfiguration(envVal, assets.ConfigFS(*envVal), "internal", "config")

	declared, err := resolveMessagingModules()
	if err != nil {
		return nil, err
	}

	broker := inframb.BrokerConfig(envVal)
	protocol := string(broker.Type)

	var operations []*msgdoc.OperationSpec
	for _, sub := range declared.Subscriptions {
		operations = append(operations, msgdoc.DescribeSubscription(
			sub.SubscriptionDoc(), protocol, []string{brokerServerName}))
	}
	for _, pub := range declared.Publications {
		operations = append(operations, msgdoc.DescribePublication(
			pub.PublicationDoc(), protocol, []string{brokerServerName}))
	}
	for _, channel := range declared.Channels {
		operations = append(operations, msgdoc.DescribeChannel(
			channel.ModuleDoc(), []string{wsServerName})...)
	}

	spec := msgdoc.Build(operations)
	spec.Info = apiinfo.MessagingInfo()
	spec.Perspective = Perspective
	spec.Servers = buildServers(envVal, broker)
	spec.Conventions = buildMessagingConventions()
	return spec, nil
}

// messagingModules is what the DI groups yield.
type messagingModules struct {
	fx.In
	Subscriptions []*mbkit.SubscriptionModule   `group:"mb_subscriptions"`
	Publications  []mbkit.DocumentedPublication `group:"mb_publications"`
	Channels      []*wskit.Module               `group:"ws_modules"`
}

// resolveMessagingModules resolves the three declaration groups with fx.
//
// It mirrors resolveHTTPModules: the connection-shaped dependencies are stubbed
// and no lifecycle runs, so nothing dials a broker or binds a port. What differs
// is only which groups are populated.
func resolveMessagingModules() (*messagingModules, error) {
	var modules messagingModules

	app := fx.New(
		fx.Provide(dikit.Constructors()...),
		fx.Provide(
			func() mb.Publish { return stubPublish },
			func() worker.Enqueuer { return stubEnqueuer{} },
		),
		fx.Populate(&modules),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		return nil, fmt.Errorf("describe: resolve messaging modules: %w", err)
	}
	return &modules, nil
}

// buildServers describes where a client connects: the WebSocket server this
// application runs, and the broker its subscriptions and publications use.
func buildServers(envVal *string, broker *mb.Config) []*msgdoc.ServerSpec {
	env := ""
	if envVal != nil {
		env = *envVal
	}

	var servers []*msgdoc.ServerSpec
	if params, err := registry.ModuleParams("default"); err == nil {
		servers = append(servers, &msgdoc.ServerSpec{
			Name:        wsServerName,
			Protocol:    "ws",
			Host:        fmt.Sprintf("localhost:%d", params.Server.Port),
			Environment: env,
			Description: "WebSocket endpoint served by this application",
		})
	}

	if server := brokerServer(broker, env); server != nil {
		servers = append(servers, server)
	}
	msgdoc.SortServers(servers)
	return servers
}

// brokerServer describes the broker endpoint, without its credentials.
//
// The username and password are stripped rather than trusted to stay out: a
// generated document is committed and shared, so a password that reaches the
// spec is a password that leaks.
func brokerServer(broker *mb.Config, env string) *msgdoc.ServerSpec {
	if broker == nil {
		return nil
	}
	protocol, host := splitBrokerURL(broker.URL)
	if protocol == "" {
		protocol = string(broker.Type)
	}
	return &msgdoc.ServerSpec{
		Name:        brokerServerName,
		Protocol:    protocol,
		Host:        host,
		Environment: env,
		Description: fmt.Sprintf("%s broker used by this application", broker.Type),
	}
}

// splitBrokerURL takes the scheme and host out of a broker URL, dropping any
// userinfo, path and query.
func splitBrokerURL(raw string) (protocol, host string) {
	if raw == "" {
		return "", ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		// Not a URL (a bare "host:port", say): keep the host part only, and let
		// the caller fall back to the broker type for the protocol.
		return "", strings.TrimSuffix(raw, "/")
	}
	return parsed.Scheme, parsed.Host
}

// buildMessagingConventions collects the rules that hold across the whole
// messaging surface, so they are stated once instead of on every operation.
func buildMessagingConventions() *msgdoc.Conventions {
	conv := &msgdoc.Conventions{
		Envelopes: []*msgdoc.EnvelopeSpec{{
			Protocol:     msgdoc.ProtocolWebSocket,
			TypeField:    wskit.EnvelopeTypeField,
			PayloadField: wskit.EnvelopePayloadField,
			Description: "Every WebSocket message is wrapped in this envelope. " +
				"The type member selects the message; the payload described by each " +
				"message is what the data member holds.",
			Example: map[string]any{
				wskit.EnvelopeTypeField:    "user.echo",
				wskit.EnvelopePayloadField: map[string]any{"message": "hello"},
			},
		}},
		DeliveryDefaults:  brokerDefaults(),
		GlobalMiddlewares: []string{"auth-upgrade"},
		CommonError:       apidoc.CommonErrorSchema(reflect.TypeOf(httpdto.Error{})),
	}

	if params, err := registry.ModuleParams("default"); err == nil {
		conv.SecuritySchemes = securitySchemes(params.Auth)
	}
	return conv
}

// brokerDefaults states the settings every subscription inherits unless it
// overrides them, so a reader is not left to guess what "not declared" means.
func brokerDefaults() map[string]string {
	defaults := mb.DefaultSubscribeConfig()
	return map[string]string{
		msgdoc.DeliveryErrorStrategy: "requeue",
		msgdoc.DeliveryAutoAck:       fmt.Sprintf("%t", defaults.AutoAck),
		msgdoc.DeliveryDurable:       fmt.Sprintf("%t", defaults.Durable),
		msgdoc.DeliveryMaxRetries:    fmt.Sprintf("%d", defaults.MaxRetries),
	}
}
