// Package wskit provides the typed message catalog for WebSocket channels:
// what a client may send, what the server sends back, and the dispatch that
// connects the two.
//
// It is the WebSocket counterpart of restkit and mbkit. A raw wsconfig.Module
// carries one opaque OnMessage handler over a stream of bytes, which tells a
// generator nothing: there is no message type, no direction, no discriminator.
// wskit replaces that single handler with a declared catalog, so msgdoc can
// reflect over it.
//
// The catalog is not documentation-only. Incoming messages reach a handler only
// through their declaration, and outgoing messages are written only through a
// declared Sender, so the described contract and the running code cannot drift
// apart.
//
// # The wire envelope
//
// Every message is wrapped in a fixed envelope:
//
//	{"type": "user.echo", "data": {"message": "hi"}}
//
// A WebSocket path carries many kinds of message in both directions, and the
// frame itself says nothing about which one this is. The envelope supplies that
// discriminator once, in one place, rather than leaving every handler to sniff
// its own payload.
package wskit

import (
	"encoding/json"

	"github.com/ensoria/validator/pkg/rule"
)

// The envelope member names. They are fixed for the whole application: a
// per-channel envelope would make every client need per-channel framing code.
const (
	// EnvelopeTypeField names the message ("user.echo").
	EnvelopeTypeField = "type"
	// EnvelopePayloadField holds the message body.
	EnvelopePayloadField = "data"
)

// incoming is one received envelope. The payload stays raw until the message
// type has been resolved, since only then is its Go type known.
type incoming struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// outgoing is one envelope being sent.
type outgoing struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// ErrorMessageName is the envelope type of the error the server sends back when
// a received message cannot be handled.
const ErrorMessageName = "error"

// Error codes carried by the error message. They match the codes restkit uses
// over HTTP, so a client branching on a code does not need a second vocabulary
// for its WebSocket connection.
const (
	// CodeNotParsable means the frame was not the expected JSON envelope.
	CodeNotParsable = "not_parsable"
	// CodeValidationFailed means the payload broke the declared rules.
	CodeValidationFailed = "validation_failed"
	// CodeUnknownMessageType means no message of that name is declared on this
	// channel.
	CodeUnknownMessageType = "unknown_message_type"
)

// Messages shown with the codes above when the failure carries no message of
// its own.
const (
	msgNotParsable        = "message is not a valid envelope"
	msgValidationFailed   = "message payload is invalid"
	msgUnknownMessageType = "unknown message type"
)

// ErrorPayload is the body of the error message, mirroring the error envelope
// restkit returns over HTTP so the two transports describe a failure the same
// way.
type ErrorPayload struct {
	// Code is the machine-readable code clients branch on.
	Code string `json:"code"`
	// Message is the human-readable explanation, in the caller's language when
	// one was negotiated at the handshake.
	Message string `json:"message"`
	// MessageType echoes the envelope type that failed, so a client with several
	// messages in flight can tell which one this refers to.
	MessageType string `json:"message_type,omitempty"`
	// FieldErrors names the payload fields that broke their rules.
	FieldErrors []FieldError `json:"field_errors,omitempty"`
}

// FieldError is one field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BehaviorSpec declares the facts about a channel that no type can express.
//
// It has no delivery section: unlike a broker, a WebSocket has no redelivery,
// no consumer group and no acknowledgement to describe. Ordering is still worth
// stating, since messages on one connection are ordered but messages across
// reconnects are not.
type BehaviorSpec struct {
	// SideEffects lists what handling messages on this channel changes.
	SideEffects []string
	// Idempotent states whether handling the same message twice is safe.
	// nil means undeclared.
	Idempotent *bool
	// Preconditions lists what must already be true to use the channel.
	Preconditions []string
	// Scopes are the permissions the connecting caller must hold.
	Scopes []string
	// Ordering states the order guarantee relied upon.
	Ordering string
}

// MessageOpts declares the documentation and validation of one message.
type MessageOpts struct {
	// Summary is the one-line description of the message.
	Summary string
	// Description is the longer explanation.
	Description string
	// FieldDocs gives the meaning of individual payload fields, keyed by field
	// path in dot / bracket notation.
	FieldDocs map[string]string
	// BodyRules validates the payload. On a received message a violation stops
	// it before the handler runs; on a sent message it stops the send.
	BodyRules []*rule.RuleSet
	// When states how a reader tells this message from the others on the
	// channel. It defaults to the envelope discriminator, so it only needs
	// setting when something further distinguishes the message.
	When string
}
