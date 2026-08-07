package msgdoc

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/mbkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// DescribeSubscription converts one subscription declaration into a receive
// operation.
//
// protocol is the broker type ("rabbitmq"), which the declaration cannot know:
// the same subscription runs against whichever broker the environment is
// configured with, so the protocol comes from the application, not the channel.
func DescribeSubscription(doc *mbkit.SubscriptionDoc, protocol string, serverNames []string) *OperationSpec {
	return &OperationSpec{
		Action:   ActionReceive,
		Protocol: protocol,
		Channel: &ChannelSpec{
			Address:     doc.Target,
			ServerNames: serverNames,
		},
		Messages: []*MessageSpec{{
			Name:        messageName(doc.MsgType, doc.Target),
			Summary:     doc.Summary,
			ContentType: rest.MediaTypeJSON,
			Payload:     payloadSchema(doc.MsgType, doc.BodyRules, doc.FieldDocs, doc.Target),
		}},
		Behavior:    subscribeBehavior(doc.Behavior, doc.Config, protocol),
		Summary:     doc.Summary,
		Description: doc.Description,
		Task:        doc.Task,
		AlsoRead:    doc.AlsoRead,
		Related:     doc.Related,
	}
}

// DescribePublication converts one publication declaration into a send
// operation.
func DescribePublication(doc *mbkit.PublicationDoc, protocol string, serverNames []string) *OperationSpec {
	return &OperationSpec{
		Action:   ActionSend,
		Protocol: protocol,
		Channel: &ChannelSpec{
			Address:     doc.Target,
			ServerNames: serverNames,
		},
		Messages: []*MessageSpec{{
			Name:        messageName(doc.MsgType, doc.Target),
			Summary:     doc.Summary,
			ContentType: rest.MediaTypeJSON,
			Payload:     payloadSchema(doc.MsgType, doc.BodyRules, doc.FieldDocs, doc.Target),
		}},
		Behavior:    publishBehavior(doc.Behavior, doc.Config, protocol),
		Summary:     doc.Summary,
		Description: doc.Description,
		Task:        doc.Task,
		AlsoRead:    doc.AlsoRead,
		Related:     doc.Related,
	}
}

// DescribeChannel converts one WebSocket channel into its operations: one
// receive and one send, each carrying the whole catalog for that direction.
//
// The two directions are separate operations because they are separate contracts
// — a client implements the send side and reads the receive side — even though
// they share a single connection.
//
// A channel with no message in a direction produces no operation for it: a send
// operation with an empty catalog would claim the server pushes messages when
// it never does.
func DescribeChannel(doc *wskit.ModuleDoc, serverNames []string) []*OperationSpec {
	if doc.Untyped {
		// Nothing is known beyond the path. The channel still appears, so the
		// reader learns it exists; the renderer shows TODO for its messages.
		return []*OperationSpec{{
			Action:      ActionReceive,
			Protocol:    ProtocolWebSocket,
			Channel:     channelOf(doc, serverNames),
			Summary:     doc.Summary,
			Description: doc.Description,
			Untyped:     true,
		}}
	}

	var operations []*OperationSpec
	if messages := channelMessages(doc.Receive, doc.Path); len(messages) > 0 {
		operations = append(operations, wsOperation(doc, ActionReceive, messages, serverNames))
	}
	if messages := channelMessages(doc.Send, doc.Path); len(messages) > 0 {
		operations = append(operations, wsOperation(doc, ActionSend, messages, serverNames))
	}
	return operations
}

// wsOperation builds one direction of a WebSocket channel.
func wsOperation(doc *wskit.ModuleDoc, action string, messages []*MessageSpec, serverNames []string) *OperationSpec {
	return &OperationSpec{
		Action:      action,
		Protocol:    ProtocolWebSocket,
		Channel:     channelOf(doc, serverNames),
		Messages:    messages,
		Behavior:    wsBehavior(doc.Behavior),
		Summary:     doc.Summary,
		Description: doc.Description,
		Task:        doc.Task,
		AlsoRead:    doc.AlsoRead,
		Related:     doc.Related,
	}
}

// channelOf builds the channel for a WebSocket path, lifting its `{name}`
// segments into parameters.
func channelOf(doc *wskit.ModuleDoc, serverNames []string) *ChannelSpec {
	return &ChannelSpec{
		Address:     doc.Path,
		Title:       doc.Summary,
		Parameters:  pathParameters(doc.Path),
		ServerNames: serverNames,
	}
}

// channelMessages converts a direction's message catalog.
func channelMessages(docs []*wskit.MessageDoc, path string) []*MessageSpec {
	messages := make([]*MessageSpec, 0, len(docs))
	for _, d := range docs {
		messages = append(messages, &MessageSpec{
			Name:        d.Name,
			Summary:     d.Summary,
			ContentType: rest.MediaTypeJSON,
			Payload:     payloadSchema(d.MsgType, d.BodyRules, d.FieldDocs, path),
			When:        d.When,
		})
	}
	return messages
}

// payloadSchema builds the message payload schema: the type, its constraints,
// the declared field meanings and a deterministic example.
func payloadSchema(msgType reflect.Type, rules []*rule.RuleSet, fieldDocs map[string]string, address string) *apidoc.Schema {
	return apidoc.BodySchema(msgType, rules, apidoc.ExampleOptions{Resource: resourceOf(address)}, fieldDocs)
}

// subscribeBehavior merges the declared behaviour with the delivery facts read
// back from the resolved subscribe configuration.
func subscribeBehavior(declared mbkit.BehaviorSpec, cfg *mb.SubscribeConfig, protocol string) *Behavior {
	return &Behavior{
		SideEffects:   declared.SideEffects,
		Idempotent:    declared.Idempotent,
		Preconditions: declared.Preconditions,
		Scopes:        declared.Scopes,
		Ordering:      declared.Ordering,
		Delivery: &DeliverySpec{
			Guarantee: declared.Delivery.Guarantee,
			Notes:     declared.Delivery.Notes,
			Resolved:  resolvedSubscribe(cfg, protocol),
		},
	}
}

// publishBehavior is subscribeBehavior for the publishing side.
func publishBehavior(declared mbkit.BehaviorSpec, cfg *mb.PublishConfig, protocol string) *Behavior {
	return &Behavior{
		SideEffects:   declared.SideEffects,
		Idempotent:    declared.Idempotent,
		Preconditions: declared.Preconditions,
		Scopes:        declared.Scopes,
		Ordering:      declared.Ordering,
		Delivery: &DeliverySpec{
			Guarantee: declared.Delivery.Guarantee,
			Notes:     declared.Delivery.Notes,
			Resolved:  resolvedPublish(cfg, protocol),
		},
	}
}

// wsBehavior converts the declared behaviour of a WebSocket channel. There is no
// delivery section: a socket has no redelivery, acknowledgement or consumer
// group to describe.
func wsBehavior(declared wskit.BehaviorSpec) *Behavior {
	return &Behavior{
		SideEffects:   declared.SideEffects,
		Idempotent:    declared.Idempotent,
		Preconditions: declared.Preconditions,
		Scopes:        declared.Scopes,
		Ordering:      declared.Ordering,
	}
}

// streamingBrokers are the broker types whose subscriptions are streams. For
// them consumer groups and offsets are what describe delivery, while queue
// settings (durable, exclusive) mean nothing — and the reverse for the others.
var streamingBrokers = map[string]bool{
	string(mb.TypeKafka):    true,
	string(mb.TypeRedpanda): true,
	string(mb.TypeKinesis):  true,
}

// resolvedSubscribe renders the settings in force as strings, keeping only those
// that mean something for this broker.
//
// The filtering is not cosmetic. mb fills every field of SubscribeConfig with a
// default, streaming ones included, so emitting them all would document a
// RabbitMQ queue as having a consumer group and a start offset — settings that
// are present in the struct and ignored by the broker.
func resolvedSubscribe(cfg *mb.SubscribeConfig, protocol string) map[string]string {
	if cfg == nil {
		return nil
	}
	resolved := map[string]string{
		DeliveryErrorStrategy: errorStrategyName(cfg.ErrorStrategy),
		DeliveryAutoAck:       strconv.FormatBool(cfg.AutoAck),
	}
	// MaxRetries only bounds the retry strategy; under any other strategy it is
	// a number that never applies.
	if cfg.ErrorStrategy == mb.ErrorStrategyRetry {
		resolved[DeliveryMaxRetries] = strconv.Itoa(cfg.MaxRetries)
	}

	if streamingBrokers[protocol] {
		resolved[DeliveryConsumerGroup] = cfg.ConsumerGroup
		resolved[DeliveryAutoCommit] = strconv.FormatBool(cfg.AutoCommit)
		resolved[DeliveryStartOffset] = offsetName(cfg.StartOffset)
		return resolved
	}

	resolved[DeliveryDurable] = strconv.FormatBool(cfg.Durable)
	resolved[DeliveryAutoDelete] = strconv.FormatBool(cfg.AutoDelete)
	resolved[DeliveryExclusive] = strconv.FormatBool(cfg.Exclusive)
	if cfg.QueueGroup != "" {
		resolved[DeliveryQueueGroup] = cfg.QueueGroup
	}
	return resolved
}

// resolvedPublish renders the publish settings in force as strings.
//
// Delivery mode, priority and expiration are message attributes of the queue
// brokers; a stream carries none of them, so a streaming publication resolves to
// nothing rather than to defaults it ignores.
func resolvedPublish(cfg *mb.PublishConfig, protocol string) map[string]string {
	if cfg == nil || streamingBrokers[protocol] {
		return nil
	}
	resolved := map[string]string{
		DeliveryDeliveryMode: deliveryModeName(cfg.DeliveryMode),
		DeliveryPriority:     strconv.Itoa(int(cfg.Priority)),
	}
	if cfg.Expiration != "" {
		resolved[DeliveryExpiration] = cfg.Expiration
	}
	return resolved
}

// The names given to the mb enums in the spec. They are spelled out here rather
// than taken from the library because a document should not print "2" for a
// delivery mode, and mb has no reason to carry documentation vocabulary.
var errorStrategyNames = map[mb.ErrorHandlingStrategy]string{
	mb.ErrorStrategyRequeue:    "requeue",
	mb.ErrorStrategyDiscard:    "discard",
	mb.ErrorStrategyDeadLetter: "dead-letter",
	mb.ErrorStrategyRetry:      "retry",
}

var offsetNames = map[mb.OffsetPosition]string{
	mb.OffsetBeginning: "beginning",
	mb.OffsetEnd:       "end",
	mb.OffsetStored:    "stored",
}

// deliveryModePersistent is the AMQP delivery mode that survives a broker
// restart; anything else is held in memory only.
const deliveryModePersistent = 2

func errorStrategyName(s mb.ErrorHandlingStrategy) string {
	if name, ok := errorStrategyNames[s]; ok {
		return name
	}
	return "unknown"
}

func offsetName(o mb.OffsetPosition) string {
	if name, ok := offsetNames[o]; ok {
		return name
	}
	return "unknown"
}

func deliveryModeName(mode uint8) string {
	if mode == deliveryModePersistent {
		return "persistent"
	}
	return "transient"
}

// messageName names a broker message after its Go type, falling back to the
// channel address for a type that has no name of its own.
func messageName(t reflect.Type, address string) string {
	if t == nil {
		return address
	}
	if name := t.Name(); name != "" {
		return name
	}
	return address
}

// pathParameters lifts the `{name}` segments of a WebSocket path into channel
// parameters.
func pathParameters(path string) []*ChannelParam {
	var params []*ChannelParam
	for _, seg := range strings.Split(path, "/") {
		if len(seg) >= 2 && strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			params = append(params, &ChannelParam{Name: seg[1 : len(seg)-1]})
		}
	}
	return params
}

// resourceOf derives the resource name used to keep generated example ids
// consistent, from a topic name ("user.created") or a path ("/ws/user").
func resourceOf(address string) string {
	address = strings.TrimPrefix(address, "/")
	for _, seg := range strings.FieldsFunc(address, func(r rune) bool {
		return r == '/' || r == '.' || r == '_' || r == '-'
	}) {
		if seg == "" || strings.HasPrefix(seg, "{") || seg == "ws" {
			continue
		}
		return singular(seg)
	}
	return ""
}

// singular naively drops a trailing "s" ("users" -> "user").
func singular(s string) string {
	s = strings.ToLower(s)
	if len(s) > 1 && strings.HasSuffix(s, "s") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}
