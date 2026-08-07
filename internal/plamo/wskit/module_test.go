package wskit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wserror"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/ensoria/websocket/pkg/wsserver"
)

const (
	testPath    = "/ws/test"
	testTimeout = 5 * time.Second
)

// echo is the message type these specs exchange.
type echo struct {
	Message string `json:"message"`
}

// reply is the declared server-to-client message.
var reply = wskit.Send[echo]("test.echo_reply", wskit.MessageOpts{Summary: "Echo reply"})

// envelope is the decoded wire form, for asserting on what actually crossed.
type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// serve boots a server for the module and returns a connected client.
// header is sent with the upgrade request, which is where a client states its
// language preference.
func serve(module *wskit.Module, header http.Header) (*websocket.Conn, func()) {
	GinkgoHelper()
	srv := wsserver.New(module.RuntimeModule())
	mux := http.NewServeMux()
	mux.HandleFunc(testPath, srv.Handler())
	ts := httptest.NewServer(mux)

	cli, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+testPath, header)
	Expect(err).NotTo(HaveOccurred())

	return cli, func() {
		_ = cli.Close()
		ts.Close()
	}
}

// send writes a raw frame, for the specs that need a malformed one.
func send(cli *websocket.Conn, raw string) {
	GinkgoHelper()
	Expect(cli.WriteMessage(websocket.TextMessage, []byte(raw))).To(Succeed())
}

// nextEnvelope reads the next frame and decodes its envelope.
func nextEnvelope(cli *websocket.Conn) envelope {
	GinkgoHelper()
	Expect(cli.SetReadDeadline(time.Now().Add(testTimeout))).To(Succeed())
	_, data, err := cli.ReadMessage()
	Expect(err).NotTo(HaveOccurred())
	var env envelope
	Expect(json.Unmarshal(data, &env)).To(Succeed())
	return env
}

// errorPayload reads the next frame and decodes it as an error message.
func errorPayload(cli *websocket.Conn) *wskit.ErrorPayload {
	GinkgoHelper()
	env := nextEnvelope(cli)
	Expect(env.Type).To(Equal(wskit.ErrorMessageName))
	var payload wskit.ErrorPayload
	Expect(json.Unmarshal(env.Data, &payload)).To(Succeed())
	return &payload
}

// echoChannel declares a channel that echoes back what it receives, applying any
// per-spec adjustments first.
func echoChannel(adjust func(*wskit.Channel)) *wskit.Module {
	channel := &wskit.Channel{
		Path: testPath,
		Receive: []*wskit.Receiver{
			wskit.Receive[echo]("test.echo", wskit.MessageOpts{
				BodyRules: []*rule.RuleSet{
					{Field: "message", Rules: []rule.Rule{vkit.Required()}},
				},
			}, func(ctx context.Context, event *wsevent.Message, msg *echo) error {
				return reply.Send(ctx, event.Conn, &echo{Message: msg.Message})
			}),
		},
		Send: []wskit.DocumentedMessage{reply},
		// The heartbeat would interleave pings with the frames under test.
		Configure: func(m *wsconfig.Module) { m.Heartbeat.Disable = true },
	}
	if adjust != nil {
		adjust(channel)
	}
	return wskit.NewModule(channel)
}

var _ = Describe("dispatch", func() {
	It("routes a declared message to its handler and replies in the envelope", func() {
		cli, cleanup := serve(echoChannel(nil), nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"hi"}}`)

		env := nextEnvelope(cli)
		Expect(env.Type).To(Equal("test.echo_reply"))
		var payload echo
		Expect(json.Unmarshal(env.Data, &payload)).To(Succeed())
		Expect(payload.Message).To(Equal("hi"))
	})

	It("gives the handler the decoded payload, not the envelope", func() {
		var got *echo
		module := echoChannel(func(c *wskit.Channel) {
			c.Receive = []*wskit.Receiver{
				wskit.Receive[echo]("test.echo", wskit.MessageOpts{},
					func(ctx context.Context, event *wsevent.Message, msg *echo) error {
						got = msg
						return reply.Send(ctx, event.Conn, msg)
					}),
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"decoded"}}`)
		nextEnvelope(cli)

		Expect(got).NotTo(BeNil())
		Expect(got.Message).To(Equal("decoded"))
	})

	It("accepts a message that carries no payload", func() {
		// {"type": "app.ping"} is a normal thing to send, and omitting an empty
		// data member is the natural way to write it.
		module := echoChannel(func(c *wskit.Channel) {
			c.Receive = []*wskit.Receiver{
				wskit.Receive[struct{}]("test.ping", wskit.MessageOpts{},
					func(ctx context.Context, event *wsevent.Message, _ *struct{}) error {
						return reply.Send(ctx, event.Conn, &echo{Message: "pong"})
					}),
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.ping"}`)

		Expect(nextEnvelope(cli).Type).To(Equal("test.echo_reply"))
	})

	Describe("a message the client got wrong", func() {
		It("answers an unknown type with an error and keeps the connection", func() {
			cli, cleanup := serve(echoChannel(nil), nil)
			defer cleanup()

			send(cli, `{"type":"test.nope","data":{}}`)

			payload := errorPayload(cli)
			Expect(payload.Code).To(Equal(wskit.CodeUnknownMessageType))
			Expect(payload.MessageType).To(Equal("test.nope"))

			// The connection has to survive: one bad message from a buggy client
			// should not cost it every other message on the socket.
			send(cli, `{"type":"test.echo","data":{"message":"still here"}}`)
			Expect(nextEnvelope(cli).Type).To(Equal("test.echo_reply"))
		})

		It("answers a frame that is not an envelope", func() {
			cli, cleanup := serve(echoChannel(nil), nil)
			defer cleanup()

			send(cli, `not json at all`)

			Expect(errorPayload(cli).Code).To(Equal(wskit.CodeNotParsable))
		})

		It("answers an envelope with no type", func() {
			cli, cleanup := serve(echoChannel(nil), nil)
			defer cleanup()

			send(cli, `{"data":{"message":"hi"}}`)

			Expect(errorPayload(cli).Code).To(Equal(wskit.CodeNotParsable))
		})

		It("names the offending field when the payload breaks its rules", func() {
			cli, cleanup := serve(echoChannel(nil), nil)
			defer cleanup()

			send(cli, `{"type":"test.echo","data":{"message":""}}`)

			payload := errorPayload(cli)
			Expect(payload.Code).To(Equal(wskit.CodeValidationFailed))
			Expect(payload.MessageType).To(Equal("test.echo"))
			Expect(payload.FieldErrors).To(HaveLen(1))
			Expect(payload.FieldErrors[0].Field).To(Equal("message"))
			Expect(payload.FieldErrors[0].Code).To(Equal("str_not_empty"))
			Expect(payload.FieldErrors[0].Message).To(Equal("this field is required"))
		})

		It("never lets an invalid payload reach the handler", func() {
			called := false
			module := echoChannel(func(c *wskit.Channel) {
				c.Receive = []*wskit.Receiver{
					wskit.Receive[echo]("test.echo", wskit.MessageOpts{
						BodyRules: []*rule.RuleSet{
							{Field: "message", Rules: []rule.Rule{vkit.Required()}},
						},
					}, func(ctx context.Context, event *wsevent.Message, msg *echo) error {
						called = true
						return nil
					}),
				}
			})
			cli, cleanup := serve(module, nil)
			defer cleanup()

			send(cli, `{"type":"test.echo","data":{"message":""}}`)
			errorPayload(cli)

			Expect(called).To(BeFalse())
		})

		It("writes the failure in the language negotiated at the handshake", func() {
			// A frame carries no headers, so the handshake is the only place the
			// caller can ever state a preference.
			cli, cleanup := serve(echoChannel(nil), http.Header{"Accept-Language": []string{"ja"}})
			defer cleanup()

			send(cli, `{"type":"test.echo","data":{"message":""}}`)

			Expect(errorPayload(cli).FieldErrors[0].Message).To(Equal("必須です"))
		})
	})

	It("closes the connection when the handler itself fails", func() {
		module := echoChannel(func(c *wskit.Channel) {
			c.Receive = []*wskit.Receiver{
				wskit.Receive[echo]("test.echo", wskit.MessageOpts{},
					func(context.Context, *wsevent.Message, *echo) error {
						return errors.New("downstream unavailable")
					}),
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"hi"}}`)

		// An application failure is not the client's fault to recover from, so
		// it follows the library's rule: a handler error closes the connection.
		Expect(cli.SetReadDeadline(time.Now().Add(testTimeout))).To(Succeed())
		_, _, err := cli.ReadMessage()
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("NewModule", func() {
	It("lets Configure set up the lifecycle handlers", func() {
		opened := false
		module := echoChannel(func(c *wskit.Channel) {
			c.Configure = func(m *wsconfig.Module) {
				m.Heartbeat.Disable = true
				m.OnOpen = func(context.Context, *wsevent.Open) error {
					opened = true
					return nil
				}
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"hi"}}`)
		nextEnvelope(cli)

		Expect(opened).To(BeTrue())
	})

	It("does not let Configure take over message handling", func() {
		hijacked := false
		module := echoChannel(func(c *wskit.Channel) {
			c.Configure = func(m *wsconfig.Module) {
				m.Heartbeat.Disable = true
				m.OnMessage = func(context.Context, *wsevent.Message) error {
					hijacked = true
					return nil
				}
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"hi"}}`)

		// Routing around the catalog is what the declarations exist to prevent,
		// so wskit installs its dispatcher after Configure has run.
		Expect(nextEnvelope(cli).Type).To(Equal("test.echo_reply"))
		Expect(hijacked).To(BeFalse())
	})

	It("lets Configure add OnMessage middlewares, which wrap the dispatcher", func() {
		wrapped := false
		module := echoChannel(func(c *wskit.Channel) {
			c.Configure = func(m *wsconfig.Module) {
				m.Heartbeat.Disable = true
				m.AddOnMessageMiddleware(func(next wsconfig.OnMessageHandler) wsconfig.OnMessageHandler {
					return func(ctx context.Context, event *wsevent.Message) error {
						wrapped = true
						return next(ctx, event)
					}
				})
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"hi"}}`)

		// Middlewares are not the exception OnMessage is: they still see every
		// frame, and the dispatcher runs as the innermost handler.
		Expect(nextEnvelope(cli).Type).To(Equal("test.echo_reply"))
		Expect(wrapped).To(BeTrue())
	})

	It("lets Configure set the other lifecycle handlers, including OnError", func() {
		var errorPhase wserror.ErrorType
		module := echoChannel(func(c *wskit.Channel) {
			c.Receive = []*wskit.Receiver{
				wskit.Receive[echo]("test.echo", wskit.MessageOpts{},
					func(context.Context, *wsevent.Message, *echo) error {
						return errors.New("downstream unavailable")
					}),
			}
			c.Configure = func(m *wsconfig.Module) {
				m.Heartbeat.Disable = true
				m.OnError = func(_ context.Context, event *wsevent.Error) error {
					errorPhase = event.ErrorType
					return nil
				}
			}
		})
		cli, cleanup := serve(module, nil)
		defer cleanup()

		send(cli, `{"type":"test.echo","data":{"message":"hi"}}`)
		Expect(cli.SetReadDeadline(time.Now().Add(testTimeout))).To(Succeed())
		_, _, err := cli.ReadMessage()
		Expect(err).To(HaveOccurred())

		Eventually(func() wserror.ErrorType { return errorPhase }).
			Should(Equal(wserror.OnMessageMiddleware))
	})

	It("keeps the declared path on the runtime module", func() {
		Expect(echoChannel(nil).RuntimeModule().Path).To(Equal(testPath))
	})
})

var _ = Describe("ModuleDoc", func() {
	It("carries the declaration and both message catalogs", func() {
		idempotent := true
		module := echoChannel(func(c *wskit.Channel) {
			c.Summary = "Test channel"
			c.Description = "Longer explanation"
			c.Task = "demo realtime"
			c.AlsoRead = []string{"workflows/realtime.md"}
			c.Related = []string{"Opened after: POST /sessions"}
			c.Behavior = wskit.BehaviorSpec{
				SideEffects: []string{"none"},
				Idempotent:  &idempotent,
				Scopes:      []string{"users:read"},
				Ordering:    "ordered within one connection",
			}
		})

		doc := module.ModuleDoc()

		Expect(doc.Path).To(Equal(testPath))
		Expect(doc.Summary).To(Equal("Test channel"))
		Expect(doc.Task).To(Equal("demo realtime"))
		Expect(doc.AlsoRead).To(Equal([]string{"workflows/realtime.md"}))
		Expect(doc.Related).To(Equal([]string{"Opened after: POST /sessions"}))
		Expect(doc.Behavior.Scopes).To(Equal([]string{"users:read"}))
		Expect(doc.Untyped).To(BeFalse())

		Expect(doc.Receive).To(HaveLen(1))
		Expect(doc.Receive[0].Name).To(Equal("test.echo"))
		Expect(doc.Receive[0].BodyRules).To(HaveLen(1))
		Expect(doc.Send).To(HaveLen(1))
		Expect(doc.Send[0].Name).To(Equal("test.echo_reply"))
	})
})

var _ = Describe("Raw", func() {
	It("marks a hand-built module as untyped rather than dropping it", func() {
		module := wskit.Raw(wsconfig.NewDefaultModule("/ws/legacy"))

		doc := module.ModuleDoc()

		// A reachable channel missing from the document is worse than one the
		// reader can see is undocumented.
		Expect(doc.Path).To(Equal("/ws/legacy"))
		Expect(doc.Untyped).To(BeTrue())
		Expect(doc.Receive).To(BeEmpty())
		Expect(doc.Send).To(BeEmpty())
	})

	It("serves the module it was given, untouched", func() {
		raw := wsconfig.NewDefaultModule("/ws/legacy")

		Expect(wskit.Raw(raw).RuntimeModule()).To(BeIdenticalTo(raw))
	})
})
