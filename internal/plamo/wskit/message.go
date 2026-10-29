package wskit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/validator/pkg/verr"
	"github.com/ensoria/websocket/pkg/wsconn"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/ensoria/websocket/pkg/wssend"
)

// MessageDoc is the non-generic view of one declared message that msgdoc reads.
type MessageDoc struct {
	Name        string
	Summary     string
	Description string
	FieldDocs   map[string]string
	// MsgType is the declared payload type, the source of the payload schema.
	MsgType   reflect.Type
	BodyRules []*rule.RuleSet
	When      string
	// Security is who may send the message, as declared. Only a received
	// message has one; nil adds nothing to the channel's declaration.
	Security *MessageSecurity
}

// DocumentedMessage is satisfied by every declared message, in either
// direction. It is what lets a channel hold its send catalog in one slice even
// though each Sender has a different payload type.
type DocumentedMessage interface {
	MessageDoc() *MessageDoc
}

// Receiver is one message a client may send, bound to the handler that takes it.
//
// Its fields are unexported and it is only built by Receive, so the payload type
// recorded for the documentation is by construction the same type the handler
// receives — they come from the one type parameter.
type Receiver struct {
	name    string
	msgType reflect.Type
	opts    ReceiveOpts
	// handle decodes the payload and calls the typed handler. The type parameter
	// is closed over here, which is what lets a channel hold receivers for
	// different payload types in a single slice.
	//
	// It returns validation failures separately from the handler's error: the
	// first is the client's fault and is answered with an error message, the
	// second is the application's and closes the connection.
	handle func(ctx context.Context, event *wsevent.Message, data json.RawMessage) (verr.ValidationErrors, error)
}

// Receive declares a message the client may send, and the handler that takes it.
//
//	wskit.Receive[dto.UserEcho]("user.echo", wskit.ReceiveOpts{
//	    Summary:   "Echo request",
//	    BodyRules: []*rule.RuleSet{{Field: "message", Rules: []rule.Rule{vkit.Required()}}},
//	}, func(ctx context.Context, event *wsevent.Message, msg *dto.UserEcho) error {
//	    return reply.Send(ctx, event.Conn, &dto.UserEchoReply{Message: msg.Message})
//	})
//
// The handler receives the decoded, validated payload, and only from a caller
// opts.Security admits. event still carries the connection and session, for
// replying and for reading the handshake.
func Receive[Msg any](name string, opts ReceiveOpts, handle func(ctx context.Context, event *wsevent.Message, msg *Msg) error) *Receiver {
	return &Receiver{
		name:    name,
		msgType: reflect.TypeFor[Msg](),
		opts:    opts,
		handle: func(ctx context.Context, event *wsevent.Message, data json.RawMessage) (verr.ValidationErrors, error) {
			msg, vErrs := vkit.JSONBody[Msg](data, opts.BodyRules...)
			if vErrs.HasErrors() {
				return vErrs, nil
			}
			return nil, handle(ctx, event, msg)
		},
	}
}

// Name is the envelope discriminator this receiver answers to.
func (r *Receiver) Name() string { return r.name }

// Security is who may send the message (nil adds nothing to the channel's
// declaration). The dispatcher checks it before the payload is decoded.
func (r *Receiver) Security() *MessageSecurity { return r.opts.Security }

// MessageDoc exposes the declaration for msgdoc.
func (r *Receiver) MessageDoc() *MessageDoc {
	return &MessageDoc{
		Name:        r.name,
		Summary:     r.opts.Summary,
		Description: r.opts.Description,
		FieldDocs:   r.opts.FieldDocs,
		MsgType:     r.msgType,
		BodyRules:   r.opts.BodyRules,
		When:        whenOf(r.name, r.opts.When),
		Security:    r.opts.Security,
	}
}

// Sender is the declared, typed way to send one message to a client.
//
// Sending goes through it rather than through wssend directly so that every
// message on the wire has a declaration behind it — and so the envelope is
// assembled in one place instead of at every call site.
type Sender[Msg any] struct {
	name string
	opts SendOpts
}

// Send declares a message the server sends, and returns the sender used to send
// it.
//
//	var reply = wskit.Send[dto.UserEchoReply]("user.echo_reply", wskit.SendOpts{
//	    Summary: "Echo reply",
//	})
//
// Hold the returned sender wherever the message is sent from: inside a receive
// handler, from OnOpen, or from a background goroutine broadcasting to a
// session. Register the same value in the channel's Send catalog, so the message
// is declared exactly once.
func Send[Msg any](name string, opts SendOpts) *Sender[Msg] {
	return &Sender[Msg]{name: name, opts: opts}
}

// Name is the envelope discriminator this sender writes.
func (s *Sender[Msg]) Name() string { return s.name }

// MessageDoc exposes the declaration for msgdoc.
func (s *Sender[Msg]) MessageDoc() *MessageDoc {
	return &MessageDoc{
		Name:        s.name,
		Summary:     s.opts.Summary,
		Description: s.opts.Description,
		FieldDocs:   s.opts.FieldDocs,
		MsgType:     reflect.TypeFor[Msg](),
		BodyRules:   s.opts.BodyRules,
		When:        whenOf(s.name, s.opts.When),
	}
}

// Send validates the message, wraps it in the envelope and writes it to conn.
//
// ctx is checked before writing: once the connection context is canceled, the
// peer is going away and the write would only fail slowly.
func (s *Sender[Msg]) Send(ctx context.Context, conn *wsconn.Conn, msg *Msg) error {
	envelope, err := s.envelope(ctx, msg)
	if err != nil {
		return err
	}
	return wssend.JSON(conn, envelope)
}

// Broadcast sends the message to every given connection, reporting the failures
// together rather than stopping at the first one — one dead connection should
// not keep the rest from receiving it.
func (s *Sender[Msg]) Broadcast(ctx context.Context, conns []*wsconn.Conn, msg *Msg) error {
	envelope, err := s.envelope(ctx, msg)
	if err != nil {
		return err
	}
	return wssend.BroadcastJSON(conns, envelope)
}

// envelope validates the message and wraps it, the part Send and Broadcast share.
func (s *Sender[Msg]) envelope(ctx context.Context, msg *Msg) (*outgoing, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("wskit: sending %s: %w", s.name, err)
	}
	if errs := vkit.Object(msg, s.opts.BodyRules...); errs.HasErrors() {
		return nil, fmt.Errorf("wskit: sending %s: payload is invalid%s", s.name, describeFailures(errs))
	}
	return &outgoing{Type: s.name, Data: msg}, nil
}

// describeFailures renders validation failures for a Go error string, naming the
// offending fields rather than only reporting that something was wrong.
func describeFailures(errs verr.ValidationErrors) string {
	out := ""
	for _, fe := range errs {
		out += "; "
		if fe.Field != "" {
			out += fe.Field + ": "
		}
		out += fe.Code
	}
	return out
}

// whenOf is the When a message is documented with: the declared one, or else
// the envelope discriminator, which is how a reader actually tells the
// messages on a channel apart.
func whenOf(name, declared string) string {
	if declared != "" {
		return declared
	}
	return fmt.Sprintf("%s is %q", EnvelopeTypeField, name)
}
