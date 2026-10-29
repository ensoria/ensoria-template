package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/app/auth"
	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/middleware"
	userws "github.com/ensoria/ensoria-template/internal/module/user/controller/ws"
	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
)

const (
	userPath    = "/ws/user"
	readTimeout = 5 * time.Second
)

// The API keys the specs connect with, each standing for a different caller.
const (
	keyNoScopes  = "key-no-scopes"
	keyAnnouncer = "key-announcer"
	keyAdmin     = "key-admin"
)

// callers is the key store behind those keys. keyAdmin holds admin alone, so
// it reaches announcements:write only through the template's scope policy.
var callers = authkit.KeyStoreFunc(func(_ context.Context, key string) (*authkit.Principal, error) {
	scopes := map[string][]string{
		keyNoScopes:  nil,
		keyAnnouncer: {"announcements:write"},
		keyAdmin:     {"admin"},
	}
	held, ok := scopes[key]
	if !ok {
		return nil, authkit.ErrKeyNotFound
	}
	return &authkit.Principal{Subject: "usr_" + key, Scheme: authkit.SchemeAPIKey, Scopes: held}, nil
})

// serveUserChannel serves the template's /ws/user channel behind the same
// guards and the same scope policy the application runs with.
func serveUserChannel() *httptest.Server {
	GinkgoHelper()

	verifier, err := authkit.NewVerifier(&appconfig.Auth{APIKeyHeader: appconfig.DefaultAPIKeyHeader}, callers, nil)
	Expect(err).NotTo(HaveOccurred())
	expander, err := auth.NewScopeExpander()
	Expect(err).NotTo(HaveOccurred())

	channel := userws.NewChannel(nil, userws.NewOnOpen(nil))
	router, err := wsApp.CreateWSRouter([]*wskit.Module{wskit.NewModule(channel.Declare())},
		verifier, middleware.ParseOrigins(""), expander, nil)
	Expect(err).NotTo(HaveOccurred())

	mux := http.NewServeMux()
	router.Register(mux)
	server := httptest.NewServer(mux)
	DeferCleanup(server.Close)
	return server
}

// connect opens a connection with the given API key; an empty key connects
// anonymously.
func connect(server *httptest.Server, key string) *websocket.Conn {
	GinkgoHelper()

	headers := http.Header{}
	if key != "" {
		headers.Set(appconfig.DefaultAPIKeyHeader, key)
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+userPath, headers)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = conn.Close() })
	return conn
}

// envelope is the wire form of a message.
type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func send(conn *websocket.Conn, raw string) {
	GinkgoHelper()
	Expect(conn.WriteMessage(websocket.TextMessage, []byte(raw))).To(Succeed())
}

// next reads the next message, decoding its payload into data.
func next(conn *websocket.Conn, data any) string {
	GinkgoHelper()
	Expect(conn.SetReadDeadline(time.Now().Add(readTimeout))).To(Succeed())
	var env envelope
	Expect(conn.ReadJSON(&env)).To(Succeed())
	Expect(json.Unmarshal(env.Data, data)).To(Succeed())
	return env.Type
}

// refusal reads the next message as the error answering messageType.
func refusal(conn *websocket.Conn, messageType string) string {
	GinkgoHelper()
	var payload wskit.ErrorPayload
	Expect(next(conn, &payload)).To(Equal(wskit.ErrorMessageName))
	Expect(payload.MessageType).To(Equal(messageType))
	return payload.Code
}

const (
	echo     = `{"type":"user.echo","data":{"message":"hi"}}`
	whoAmI   = `{"type":"user.whoami"}`
	announce = `{"type":"user.announce","data":{"message":"maintenance at noon"}}`
)

var _ = Describe("the /ws/user channel", func() {
	var server *httptest.Server
	BeforeEach(func() { server = serveUserChannel() })

	Describe("for an anonymous caller", func() {
		It("echoes, since the channel is public", func() {
			conn := connect(server, "")

			send(conn, echo)

			var reply dto.UserEchoReply
			Expect(next(conn, &reply)).To(Equal(userws.EchoReplyMessage))
			Expect(reply.Message).To(Equal("hi"))
		})

		// Signing in is what these two messages add to the channel, so an
		// anonymous caller is told to authenticate — and keeps the connection.
		It("refuses the messages that need a caller as unauthenticated, keeping the connection", func() {
			conn := connect(server, "")

			send(conn, whoAmI)
			Expect(refusal(conn, userws.WhoAmIMessage)).To(Equal(wskit.CodeUnauthenticated))
			send(conn, announce)
			Expect(refusal(conn, userws.AnnounceMessage)).To(Equal(wskit.CodeUnauthenticated))

			send(conn, echo)
			var reply dto.UserEchoReply
			Expect(next(conn, &reply)).To(Equal(userws.EchoReplyMessage))
		})
	})

	Describe("for a signed-in caller without announcements:write", func() {
		It("says who the caller is", func() {
			conn := connect(server, keyNoScopes)

			send(conn, whoAmI)

			var reply dto.UserWhoAmIReply
			Expect(next(conn, &reply)).To(Equal(userws.WhoAmIReplyMessage))
			Expect(reply.Subject).To(Equal("usr_" + keyNoScopes))
			Expect(reply.Scheme).To(Equal(authkit.SchemeAPIKey))
		})

		It("refuses an announcement as forbidden", func() {
			conn := connect(server, keyNoScopes)

			send(conn, announce)

			Expect(refusal(conn, userws.AnnounceMessage)).To(Equal(wskit.CodeForbidden))
		})
	})

	Describe("an announcement", func() {
		It("reaches every connection on the channel, anonymous listeners included", func() {
			listener := connect(server, "")
			announcer := connect(server, keyAnnouncer)

			send(announcer, announce)

			for _, conn := range []*websocket.Conn{announcer, listener} {
				var got dto.UserAnnouncement
				Expect(next(conn, &got)).To(Equal(userws.AnnouncementMessage))
				Expect(got.Message).To(Equal("maintenance at noon"))
			}
		})

		// The template's policy lets admin stand for announcements:write. A
		// message judged without that policy would refuse admin quietly.
		It("is accepted from a caller holding only admin", func() {
			announcer := connect(server, keyAdmin)

			send(announcer, announce)

			var got dto.UserAnnouncement
			Expect(next(announcer, &got)).To(Equal(userws.AnnouncementMessage))
		})
	})
})
