package describe

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	assets "github.com/ensoria/ensoria-template"
	"github.com/ensoria/ensoria-template/internal/app/apiinfo"
	authApp "github.com/ensoria/ensoria-template/internal/app/auth"
	httpdto "github.com/ensoria/ensoria-template/internal/app/http/dto"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	inframb "github.com/ensoria/ensoria-template/internal/infra/mb"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/mb/pkg/mb"
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
	if err := registry.InitializeConfiguration(*envVal, assets.ConfigFS(*envVal), "internal", "config"); err != nil {
		return nil, fmt.Errorf("app initialization error: %w", err)
	}

	declared, err := resolveMessagingModules()
	if err != nil {
		return nil, err
	}

	// An application may run without a broker at all, in which case only the
	// WebSocket side is described rather than the whole run failing. A
	// configuration that cannot be read is a different matter and is reported.
	//
	// The registry is named explicitly because this call is outside the graph:
	// the one the constructors are handed is provided in stubs.go, and both have
	// to be the registry InitializeConfiguration filled above.
	broker, err := inframb.BrokerConfig(registry.DefaultRegistry())
	if err != nil {
		return nil, err
	}
	protocol := ""
	brokerServers := []string(nil)
	if broker != nil {
		protocol = string(broker.Type)
		brokerServers = []string{brokerServerName}
	}

	// Every broker message carries the headers that say who started the work
	// it belongs to, whichever way it travels.
	headers := initiatorHeaderSchema(authApp.InitiatorClaimNames())
	// Read once here, with the registry known to be filled, and handed on:
	// the builders below can then be exercised without one.
	params, err := registry.ModuleParams("default")
	if err != nil {
		params = nil
	}

	var operations []*msgdoc.OperationSpec
	for _, sub := range declared.Subscriptions {
		operations = append(operations, withHeaders(msgdoc.DescribeSubscription(
			sub.SubscriptionDoc(), protocol, brokerServers), headers))
	}
	for _, pub := range declared.Publications {
		operations = append(operations, withHeaders(msgdoc.DescribePublication(
			pub.PublicationDoc(), protocol, brokerServers), headers))
	}
	for _, channel := range declared.Channels {
		operations = append(operations, msgdoc.DescribeChannel(
			channel.ModuleDoc(), []string{wsServerName}, verifiableSchemes(params))...)
	}

	spec := msgdoc.Build(operations)
	spec.Info = apiinfo.MessagingInfo()
	spec.Perspective = Perspective
	spec.Servers = buildServers(envVal, broker)
	spec.Conventions = buildMessagingConventions(params)
	return spec, nil
}

// withHeaders puts the header schema on every message of a broker operation.
func withHeaders(op *msgdoc.OperationSpec, headers *apidoc.Schema) *msgdoc.OperationSpec {
	for _, m := range op.Messages {
		m.Headers = headers
	}
	return op
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
// It mirrors resolveHTTPModules: the same stubs.go list stands in for the
// infrastructure and no lifecycle runs, so nothing dials a broker or binds a
// port. What differs is only which groups are populated.
func resolveMessagingModules() (*messagingModules, error) {
	var modules messagingModules

	app := fx.New(
		fx.Provide(dikit.Constructors()...),
		fx.Provide(stubs()...),
		fx.Populate(&modules),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		return nil, withStubHint(fmt.Errorf("describe: resolve messaging modules: %w", err))
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
		protocol, host := websocketAddress(params.Server.PublicURLOrDefault())
		servers = append(servers, &msgdoc.ServerSpec{
			Name:        wsServerName,
			Protocol:    protocol,
			Host:        host,
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

// websocketAddress turns the address this deployment answers on into the pair
// AsyncAPI wants for a server: the protocol and the host.
//
// The scheme is translated rather than carried: a WebSocket connection to an
// application served over https is wss, and publishing ws there tells a browser
// to open a connection it will refuse as mixed content. A path prefix is dropped
// because AsyncAPI puts the path on the channel, and this application's channels
// declare their own.
func websocketAddress(publicURL string) (protocol, host string) {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host == "" {
		return wsProtocol, publicURL
	}
	if parsed.Scheme == "https" {
		return wssProtocol, parsed.Host
	}
	return wsProtocol, parsed.Host
}

// The two WebSocket schemes, named so the translation above reads as one.
const (
	wsProtocol  = "ws"
	wssProtocol = "wss"
)

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
		Security:    brokerSecurity(broker),
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
//
// params is the configuration being described, or nil when it could not be
// read; what depends on it (the credential kinds, the recheck interval) is then
// left out rather than guessed.
func buildMessagingConventions(params *appconfig.Parameters) *msgdoc.Conventions {
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
		DeliveryDefaults: brokerDefaults(),
		// Taken from the chain the router builds rather than restated here, so
		// the two cannot say different things.
		GlobalMiddlewares: wsApp.UpgradeGuardNames(),
		CommonError:       apidoc.CommonErrorSchema(reflect.TypeOf(httpdto.Error{})),
		// The scope policy the running application enforces. It is not
		// configuration, so it does not wait for the registry below.
		ScopeImplications: authApp.ScopeImplicationTable(),
		Initiator:         initiatorConvention(),
		WebSocket:         websocketConvention(0),
	}

	if params != nil {
		conv.SecuritySchemes = securitySchemes(params.Auth)
		conv.WebSocket = websocketConvention(params.Auth.WebSocket.RecheckInterval)
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
