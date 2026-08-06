// Package msgdoc builds a format-neutral model of the application's messaging
// surface (MessagingSpec) from the typed declarations in mbkit (message broker
// subscriptions and publications) and wskit (WebSocket channels).
//
// It is the messaging counterpart of apidoc: describe writes it out as JSON and
// encli renders it into AsyncAPI. Keeping the model neutral — rather than
// speaking AsyncAPI directly — is what lets a second renderer (DocAI Messaging)
// be added later without touching the declarations or the describe program.
//
// Payload schemas are not reinvented here: msgdoc imports apidoc and reuses its
// Schema tree, constraints and examples. The dependency is one-way (msgdoc ->
// apidoc), because a message payload is described exactly like an HTTP body.
package msgdoc

import "github.com/ensoria/ensoria-template/internal/plamo/apidoc"

// MessagingSpec is the neutral model of the whole messaging surface.
type MessagingSpec struct {
	Info *apidoc.Info `json:"info,omitempty"`
	// Perspective names the application these operations belong to. Every Action
	// is written from its point of view, so a reader always knows whose "send"
	// this is. Without it, a receive operation is ambiguous between "this app
	// consumes it" and "someone consumes what this app emits".
	Perspective string           `json:"perspective,omitempty"`
	Servers     []*ServerSpec    `json:"servers,omitempty"`
	Operations  []*OperationSpec `json:"operations"`
	Conventions *Conventions     `json:"conventions,omitempty"`
}

// Action is the direction of an operation, always from the application's own
// point of view (see MessagingSpec.Perspective).
const (
	// ActionSend is this application producing a message: publishing to a broker
	// or pushing to a WebSocket client.
	ActionSend = "send"
	// ActionReceive is this application consuming a message: a broker
	// subscription or a message arriving from a WebSocket client.
	ActionReceive = "receive"
)

// ProtocolWebSocket is the Protocol value for WebSocket operations. Broker
// operations use the broker type as their protocol ("rabbitmq", "kafka", ...),
// which is why this is a plain string rather than an enum.
const ProtocolWebSocket = "websocket"

// ServerSpec is one endpoint a client can connect to: the WebSocket server, or
// the broker instance carrying a topic.
//
// Host never carries credentials. A generated document is committed and shared,
// so a password that reaches this struct is a password that leaks; the builder
// strips them rather than trusting every renderer to remember to.
type ServerSpec struct {
	// Name identifies the server and is what ChannelSpec.ServerNames refers to.
	Name string `json:"name"`
	// Protocol is the wire protocol ("ws", "amqp", "kafka", ...).
	Protocol string `json:"protocol"`
	// ProtocolVersion pins the protocol revision when it matters ("0.9.1").
	ProtocolVersion string `json:"protocol_version,omitempty"`
	// Host is host[:port], without scheme, userinfo or path.
	Host string `json:"host"`
	// Pathname is the path part, when the protocol has one (a WebSocket vhost).
	Pathname string `json:"pathname,omitempty"`
	// Environment is the config environment this server was resolved from
	// ("local", "production"), so several environments can be listed at once.
	Environment string `json:"environment,omitempty"`
	Description string `json:"description,omitempty"`
}

// OperationSpec is one direction of one channel: what this application sends to
// it, or receives from it.
type OperationSpec struct {
	// Action is ActionSend or ActionReceive.
	Action string `json:"action"`
	// Protocol is ProtocolWebSocket or the broker type.
	Protocol string       `json:"protocol"`
	Channel  *ChannelSpec `json:"channel"`
	// Messages is the catalog of messages this operation carries. A broker
	// operation carries exactly one; a WebSocket operation may carry several,
	// discriminated by the envelope (see MessageSpec.When).
	Messages []*MessageSpec `json:"messages,omitempty"`
	// Behavior holds the facts no type can express. nil means nothing was
	// declared, which renderers surface as TODO.
	Behavior    *Behavior `json:"behavior,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Description string    `json:"description,omitempty"`
	// Task is the client-intent label, matching apidoc's INDEX Task column.
	Task     string   `json:"task,omitempty"`
	AlsoRead []string `json:"also_read,omitempty"`
	Related  []string `json:"related,omitempty"`
	// Untyped marks an operation declared through a raw handler or a raw
	// wsconfig.Module, where no payload type is known. It is the messaging
	// equivalent of apidoc's untyped controller: the operation still appears, so
	// the reader learns the channel exists, but the renderer shows TODO for its
	// messages instead of silently omitting it.
	Untyped bool `json:"untyped,omitempty"`
}

// ChannelSpec is the addressable thing messages flow through: a broker topic or
// queue, or a WebSocket path.
type ChannelSpec struct {
	// Address is the target as written in the declaration ("hello_world",
	// "/ws/user"). Path parameters keep their `{name}` form.
	Address     string          `json:"address"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  []*ChannelParam `json:"parameters,omitempty"`
	// ServerNames lists the ServerSpec.Name values that carry this channel.
	// Empty means every declared server.
	ServerNames []string `json:"server_names,omitempty"`
}

// ChannelParam is one `{name}` in a channel address.
type ChannelParam struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// MessageSpec is one message shape an operation can carry.
type MessageSpec struct {
	// Name is the message identifier. For WebSocket it is the envelope's type
	// discriminator ("user.echo"); for a broker it names the payload type.
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
	// ContentType defaults to application/json when empty.
	ContentType string `json:"content_type,omitempty"`
	// Payload is the decoded message body — what the handler actually receives,
	// not the envelope around it. Conventions.Envelopes describes the wrapper
	// once, rather than repeating it in every message.
	Payload *apidoc.Schema `json:"payload,omitempty"`
	// Headers describes broker metadata carried alongside the payload. Usually
	// nil for WebSocket, which has no per-message header channel.
	Headers *apidoc.Schema `json:"headers,omitempty"`
	// When states how a reader tells this message from the others on the same
	// channel ("envelope type is user.echo"). Only meaningful when the operation
	// carries more than one message.
	When string `json:"when,omitempty"`
}

// Behavior holds the facts about an operation that no type can express.
//
// Every field distinguishes "declared as nothing" from "nobody wrote it down":
// a nil or empty field is unknown and renders as TODO, while an explicit None
// entry means the author considered it and there is genuinely nothing. That
// distinction is what stops a generated document from quietly reading as a
// promise nobody made.
type Behavior struct {
	SideEffects []string `json:"side_effects,omitempty"`
	// Idempotent states whether redelivering the same message is safe. nil is
	// undeclared — which matters more here than over HTTP, since at-least-once
	// brokers redeliver whether or not the handler is ready for it.
	Idempotent    *bool    `json:"idempotent,omitempty"`
	Preconditions []string `json:"preconditions,omitempty"`
	// Scopes are the permissions a caller must hold to use this channel.
	Scopes   []string      `json:"scopes,omitempty"`
	Delivery *DeliverySpec `json:"delivery,omitempty"`
	// Ordering states what order guarantee the operation relies on
	// ("per-partition by user id", "none").
	Ordering string `json:"ordering,omitempty"`
}

// None is the explicit "there is nothing here" entry for the Behavior lists.
// Writing it down is what separates a considered "no side effects" from an
// undeclared one.
const None = "none"

// DeliverySpec records both what the author promised about delivery and what the
// runtime options actually resolve to.
//
// Both halves are kept because they answer different questions and can disagree.
// Guarantee is a claim a reader can act on; Resolved is the configuration that
// is really in force. A renderer that showed only the prose would hide a
// subscription whose options contradict it, and one that showed only the
// resolved options would make the reader infer the guarantee themselves.
type DeliverySpec struct {
	// Guarantee is the declared delivery semantics ("at-least-once").
	Guarantee string `json:"guarantee,omitempty"`
	// Notes are further declared facts (retry policy, dead-letter destination).
	Notes []string `json:"notes,omitempty"`
	// Resolved holds the structured facts read back from the runtime options
	// after they were applied to the broker defaults, keyed by the Delivery*
	// constants. It is a map rather than a struct because the meaningful keys
	// differ per broker — consumer_group means nothing to RabbitMQ, durable
	// means nothing to Kafka — and a struct would present every broker's
	// settings as if they all applied.
	Resolved map[string]string `json:"resolved,omitempty"`
}

// Keys of DeliverySpec.Resolved. They are named after the mb config fields they
// come from, so a reader can trace any value back to the option that set it.
const (
	DeliveryErrorStrategy = "error_strategy"
	DeliveryMaxRetries    = "max_retries"
	DeliveryAutoAck       = "auto_ack"
	DeliveryQueueGroup    = "queue_group"
	DeliveryDurable       = "durable"
	DeliveryAutoDelete    = "auto_delete"
	DeliveryExclusive     = "exclusive"
	DeliveryConsumerGroup = "consumer_group"
	DeliveryAutoCommit    = "auto_commit"
	DeliveryStartOffset   = "start_offset"
	DeliveryDeliveryMode  = "delivery_mode"
	DeliveryPriority      = "priority"
	DeliveryExpiration    = "expiration"
)

// Conventions holds the rules that hold across the whole messaging surface, so
// they are stated once instead of being repeated on every operation.
type Conventions struct {
	// Envelopes describes the fixed wrapper each protocol puts around a payload.
	Envelopes []*EnvelopeSpec `json:"envelopes,omitempty"`
	// DeliveryDefaults are the broker defaults every subscription inherits
	// unless it overrides them, keyed by the Delivery* constants.
	DeliveryDefaults map[string]string `json:"delivery_defaults,omitempty"`
	// GlobalMiddlewares names what runs on every connection before any handler
	// (the WebSocket upgrade credential check, for one).
	GlobalMiddlewares []string `json:"global_middlewares,omitempty"`
	// SecuritySchemes are the credential kinds a client may present, reused from
	// apidoc so HTTP and messaging describe the same schemes the same way.
	SecuritySchemes []apidoc.SecurityScheme `json:"security_schemes,omitempty"`
	// CommonError is the error payload shape shared across the surface.
	CommonError *apidoc.Schema `json:"common_error,omitempty"`
}

// EnvelopeSpec describes the fixed wrapper a protocol puts around every payload.
type EnvelopeSpec struct {
	// Protocol is the protocol this envelope applies to (ProtocolWebSocket).
	Protocol string `json:"protocol"`
	// TypeField is the envelope member naming the message ("type"), and
	// PayloadField the member holding the payload ("data").
	TypeField    string `json:"type_field,omitempty"`
	PayloadField string `json:"payload_field,omitempty"`
	Description  string `json:"description,omitempty"`
	// Example is a rendered envelope, so a reader sees the real wire shape
	// rather than assembling it from the field names.
	Example any `json:"example,omitempty"`
}
