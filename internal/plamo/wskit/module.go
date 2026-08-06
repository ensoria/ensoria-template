package wskit

import (
	"context"
	"encoding/json"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/validator/pkg/verr"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/ensoria/websocket/pkg/wssend"
)

// Channel is the typed declaration of one WebSocket path and the messages that
// flow over it in both directions.
//
// Build the runtime module from it with NewModule; a Channel on its own serves
// no traffic.
type Channel struct {
	// Path is the route the client connects to ("/ws/user"). Required.
	Path string

	// --- Prose that cannot be derived from the types ---

	// Summary is the one-line description of what the channel is for.
	Summary string
	// Description is the longer explanation.
	Description string
	// Task is the intent label shown in generated indexes (1-3 words).
	Task string
	// AlsoRead lists further documents to read with this one.
	AlsoRead []string
	// Related declares operations that come before or after this one.
	Related []string

	// --- The message catalog ---

	// Receive lists the messages a client may send, each bound to its handler.
	//
	// This is where OnMessage handling is written. wskit installs a single
	// OnMessage on the underlying wsconfig.Module: it parses the envelope, looks
	// the message up by its type, decodes and validates the payload, and then
	// calls the handler passed to the matching Receive. So the handle function
	// given to Receive is what ultimately runs as OnMessage — one per message
	// name, instead of one handler switching over raw bytes.
	//
	// A message whose type is not declared here never reaches application code:
	// the client gets an unknown_message_type error and the connection stays
	// open.
	Receive []*Receiver
	// Send lists the messages the server sends. Register the same Sender values
	// the handlers write through, so every message on the wire is declared once
	// and the catalog cannot fall behind the code.
	Send []DocumentedMessage

	// --- Behaviour ---

	// Behavior declares side effects, idempotency, scopes and ordering.
	Behavior BehaviorSpec

	// Configure adjusts the underlying wsconfig.Module. Everything the module
	// offers except message handling is set here: OnOpen, OnClose, OnError,
	// their middlewares, OnPong, the heartbeat, the dispatch queue, the read
	// limit and the close timeout.
	//
	//	Configure: func(m *wsconfig.Module) {
	//	    m.OnOpen = onOpen.Handler()
	//	    m.OnClose = onClose.Handler()
	//	    m.OnError = onError.Handler()
	//	    m.AddOnMessageMiddleware(LogOnMessage)
	//	    m.Heartbeat.PongWait = 30 * time.Second
	//	}
	//
	// OnMessage is the one exception: do not set it here. Message handling is
	// declared per message in Receive, and the handler given to Receive is what
	// runs when that message arrives. wskit installs its own OnMessage after
	// Configure has run, so an OnMessage assigned here is silently replaced —
	// routing messages around the catalog would defeat the declarations this
	// type exists to enforce.
	//
	// OnMessage middlewares are unaffected: they wrap the dispatcher, so they
	// still see every incoming frame and can be added here as usual.
	Configure func(*wsconfig.Module)
}

// Module is the runtime form of a Channel: the wsconfig.Module the router
// serves, and the documentation msgdoc reads, from one declaration.
//
// Its fields are unexported and it is only built by NewModule or Raw, so a
// Channel cannot be registered in the DI group by mistake.
type Module struct {
	runtime *wsconfig.Module
	doc     *ModuleDoc
}

// ModuleDoc is the non-generic view of one channel that msgdoc reads.
type ModuleDoc struct {
	Path        string
	Summary     string
	Description string
	Task        string
	AlsoRead    []string
	Related     []string
	Behavior    BehaviorSpec
	Receive     []*MessageDoc
	Send        []*MessageDoc
	// Untyped marks a channel served by a raw wsconfig.Module, where no message
	// types are known. The channel still appears in the generated document — a
	// reachable endpoint missing from the documentation is worse than one marked
	// TODO — but its messages cannot be described.
	Untyped bool
}

// NewModule turns a typed declaration into the module the DI group carries.
func NewModule(channel *Channel) *Module {
	runtime := wsconfig.NewDefaultModule(channel.Path)
	if channel.Configure != nil {
		channel.Configure(runtime)
	}

	receivers := make(map[string]*Receiver, len(channel.Receive))
	for _, r := range channel.Receive {
		receivers[r.name] = r
	}
	// Installed last: whatever Configure did, messages go through the catalog.
	runtime.OnMessage = dispatch(receivers)

	return &Module{runtime: runtime, doc: channelDoc(channel)}
}

// Raw wraps a hand-built wsconfig.Module so it can sit in the same DI group.
//
// Its messages are undocumented, by construction — that is what "raw" means
// here. Prefer NewModule; use this only for a channel whose framing genuinely
// is not the envelope, and accept that the generated document shows TODO for it.
func Raw(m *wsconfig.Module) *Module {
	return &Module{
		runtime: m,
		doc:     &ModuleDoc{Path: m.Path, Untyped: true},
	}
}

// RuntimeModule is the wsconfig.Module the WebSocket router serves.
func (m *Module) RuntimeModule() *wsconfig.Module { return m.runtime }

// ModuleDoc exposes the declaration for msgdoc.
func (m *Module) ModuleDoc() *ModuleDoc { return m.doc }

// channelDoc builds the documentation view of a declared channel.
func channelDoc(channel *Channel) *ModuleDoc {
	doc := &ModuleDoc{
		Path:        channel.Path,
		Summary:     channel.Summary,
		Description: channel.Description,
		Task:        channel.Task,
		AlsoRead:    channel.AlsoRead,
		Related:     channel.Related,
		Behavior:    channel.Behavior,
	}
	for _, r := range channel.Receive {
		doc.Receive = append(doc.Receive, r.MessageDoc())
	}
	for _, s := range channel.Send {
		doc.Send = append(doc.Send, s.MessageDoc())
	}
	return doc
}

// dispatch builds the OnMessage handler: parse the envelope, find the declared
// message, decode and validate the payload, then call its handler.
//
// A message the client got wrong is answered with an error message and the
// connection stays open. One connection carries many messages, and dropping all
// of them because one was malformed would turn a client-side bug into a
// reconnect loop. An error from the handler itself is different: it is returned
// unchanged, and the library closes the connection.
func dispatch(receivers map[string]*Receiver) wsconfig.OnMessageHandler {
	return func(ctx context.Context, event *wsevent.Message) error {
		var envelope incoming
		if err := json.Unmarshal(event.MessageData, &envelope); err != nil {
			return replyError(event, &ErrorPayload{Code: CodeNotParsable, Message: msgNotParsable})
		}
		if envelope.Type == "" {
			return replyError(event, &ErrorPayload{Code: CodeNotParsable, Message: msgNotParsable})
		}

		receiver, ok := receivers[envelope.Type]
		if !ok {
			return replyError(event, &ErrorPayload{
				Code:        CodeUnknownMessageType,
				Message:     msgUnknownMessageType,
				MessageType: envelope.Type,
			})
		}

		vErrs, err := receiver.handle(ctx, event, payloadOf(envelope))
		if vErrs.HasErrors() {
			return replyError(event, validationPayload(event, envelope.Type, vErrs))
		}
		return err
	}
}

// payloadOf returns the envelope's payload, substituting JSON null when the
// member is absent.
//
// A message that carries no data of its own ({"type": "app.ping"}) is a normal
// thing to send, and omitting the empty member is the natural way to write it.
// Without this, that message would be reported as unparsable — blaming the
// client for a payload it was right not to send.
func payloadOf(envelope incoming) json.RawMessage {
	if len(envelope.Data) == 0 {
		return json.RawMessage("null")
	}
	return envelope.Data
}

// validationPayload turns neutral validation failures into the error message
// body, in the language negotiated at the handshake.
func validationPayload(event *wsevent.Message, messageType string, vErrs verr.ValidationErrors) *ErrorPayload {
	langs := vkit.PreferredLangs(handshakeLanguage(event))
	payload := &ErrorPayload{
		Code:        CodeValidationFailed,
		Message:     msgValidationFailed,
		MessageType: messageType,
	}
	for _, fe := range vErrs {
		msg := vkit.PickMessage(fe.Messages, langs)
		if fe.Field == "" {
			// A failure with no field is about the payload as a whole (it did
			// not parse), so it belongs in the top-level message.
			payload.Code = fe.Code
			payload.Message = msg
			continue
		}
		payload.FieldErrors = append(payload.FieldErrors, FieldError{
			Field:   fe.Field,
			Code:    fe.Code,
			Message: msg,
		})
	}
	return payload
}

// handshakeLanguage reads Accept-Language from the upgrade request.
//
// A WebSocket frame carries no headers, so the handshake is the only place the
// caller ever states a language preference; it applies for the whole connection.
func handshakeLanguage(event *wsevent.Message) string {
	if event.Session == nil {
		return ""
	}
	handshake := event.Session.Handshake()
	if handshake == nil {
		return ""
	}
	lang, _ := handshake.Header("Accept-Language")
	return lang
}

// replyError sends the error message back to the client.
//
// It returns nil even when the write fails: the connection is already in
// trouble, and returning an error here would close it on the client's behalf,
// which is what keeping the connection open was meant to avoid. The failing
// write surfaces on the next read instead.
func replyError(event *wsevent.Message, payload *ErrorPayload) error {
	_ = wssend.JSON(event.Conn, &outgoing{Type: ErrorMessageName, Data: payload})
	return nil
}
