package wskit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/ensoria/websocket/pkg/wsserver"
)

// recheckInterval is short so that the specs see several rechecks in well
// under a second.
const recheckInterval = 50 * time.Millisecond

// answers is a verifier whose answer to a recheck a spec changes while the
// connection is open, the way a key store's or a session store's would.
type answers struct {
	mu        sync.Mutex
	principal *authkit.Principal
	err       error
	calls     int
}

func (a *answers) answer(principal *authkit.Principal, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.principal, a.err = principal, err
}

func (a *answers) asked() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func (a *answers) Verify(*rest.Request) (*authkit.VerifyResult, error) {
	return &authkit.VerifyResult{}, nil
}

func (a *answers) Recheck(context.Context, *authkit.Credential) (*authkit.Principal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	return a.principal, a.err
}

func (a *answers) Schemes() []string { return nil }

// verifiedAs puts caller and credential on the handshake, the way the
// authentication guard does before the upgrade.
func verifiedAs(caller *authkit.Principal, credential *authkit.Credential) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		if caller != nil {
			ctx := authkit.WithPrincipal(r.Context(), caller)
			r.SetContext(authkit.WithCredential(ctx, credential))
		}
		return nil
	}
}

// watchedServer serves module behind the authentication stand-in and the
// watch, in the order the router puts them: who the caller is, the channel's
// check, then Track.
func watchedServer(module *wskit.Module, watch *wskit.CallerWatch, caller *authkit.Principal, credential *authkit.Credential) *httptest.Server {
	GinkgoHelper()

	rm := module.RuntimeModule()
	rm.HTTPMiddlewares = append([]rest.Handler{verifiedAs(caller, credential), watch.Track()}, rm.HTTPMiddlewares...)
	watch.Install(rm, module.Security())

	mux := http.NewServeMux()
	mux.HandleFunc(rm.Path, wsserver.New(rm).Handler())
	server := httptest.NewServer(mux)
	DeferCleanup(server.Close)
	return server
}

// dialWatched connects to a server watchedServer started.
func dialWatched(server *httptest.Server) *websocket.Conn {
	GinkgoHelper()

	cli, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+testPath, nil)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = cli.Close() })
	return cli
}

// newWatch builds a watch over verifier that rechecks every recheckInterval.
func newWatch(verifier authkit.Verifier, scopes authkit.ScopeExpander) *wskit.CallerWatch {
	GinkgoHelper()

	watch, err := wskit.NewCallerWatch(verifier, scopes, recheckInterval)
	Expect(err).NotTo(HaveOccurred())
	return watch
}

// closeCode reads until the server ends the connection and returns the code
// it ended it with.
func closeCode(cli *websocket.Conn) int {
	GinkgoHelper()

	Expect(cli.SetReadDeadline(time.Now().Add(testTimeout))).To(Succeed())
	for {
		_, _, err := cli.ReadMessage()
		if err == nil {
			continue
		}
		var closed *websocket.CloseError
		Expect(errors.As(err, &closed)).To(BeTrue(), "the connection ended without a close frame: %v", err)
		return closed.Code
	}
}

// stillOpen asserts that the connection still answers a message.
func stillOpen(cli *websocket.Conn) {
	GinkgoHelper()

	send(cli, `{"type":"test.echo","data":{"message":"still here"}}`)
	Expect(nextEnvelope(cli).Type).To(Equal("test.echo_reply"))
}

// lockedLog is a log destination the server's goroutines and the spec share.
type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// typed returns the records carrying the log type t.
func (l *lockedLog) typed(t string) []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil && record["type"] == t {
			records = append(records, record)
		}
	}
	return records
}

// capturingLog sends every record to a buffer for the rest of the spec.
func capturingLog() *lockedLog {
	out := &lockedLog{}
	previous := loggear.GetLogger()
	loggear.SetLogger(loggear.NewSlogLogger(loggear.WithOutput(out)))
	DeferCleanup(func() { loggear.SetLogger(previous) })
	return out
}

var _ = Describe("CallerWatch", func() {
	sessionCaller := func(scopes ...string) *authkit.Principal {
		return &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeSession, Scopes: scopes}
	}

	// Connections opened together — every client reconnecting after a deploy
	// — would otherwise ask the store together on every interval. Each wait is
	// shortened at random, never lengthened, so a revocation still reaches
	// every connection within one interval.
	It("waits a little less than the interval, by a different amount each time", func() {
		watch := newWatch(&answers{}, nil)

		waits := map[time.Duration]bool{}
		for range 200 {
			wait := watch.NextWait()
			Expect(wait).To(BeNumerically(">", recheckInterval*4/5))
			Expect(wait).To(BeNumerically("<=", recheckInterval))
			waits[wait] = true
		}
		Expect(len(waits)).To(BeNumerically(">", 1))
	})

	It("refuses an interval that is not positive", func() {
		_, err := wskit.NewCallerWatch(&answers{}, nil, 0)

		Expect(err).To(HaveOccurred())
	})

	Describe("a token", func() {
		// Nothing can end a token before its exp, and the connection must
		// not outlive it: so it ends exactly then, with nobody asked.
		It("ends the connection with 4401 when the token expires, without asking anyone", func() {
			verifier := &answers{}
			caller := &authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT}
			server := watchedServer(echoChannel(nil), newWatch(verifier, nil),
				caller, authkit.TokenCredential(time.Now().Add(300*time.Millisecond)))
			cli := dialWatched(server)

			stillOpen(cli)

			Expect(closeCode(cli)).To(Equal(int(wskit.CloseCredentialInvalid)))
			Expect(verifier.asked()).To(BeZero())
		})
	})

	Describe("a session or an API key", func() {
		// A session keeps the claims of the token it was traded for, exp
		// included; that exp is the token's, and must not end the session's
		// connection.
		It("is not ended by an exp among its claims", func() {
			verifier := &answers{}
			caller := sessionCaller()
			caller.Claims = authkit.Claims{"exp": float64(time.Now().Add(100 * time.Millisecond).Unix())}
			verifier.answer(caller, nil)
			server := watchedServer(echoChannel(nil), newWatch(verifier, nil), caller, authkit.SessionCredential("sid"))
			cli := dialWatched(server)

			time.Sleep(1100 * time.Millisecond)

			stillOpen(cli)
		})

		It("ends the connection with 4401 once the store says the credential is gone", func() {
			verifier := &answers{}
			verifier.answer(nil, authkit.ErrInvalidCredential)
			log := capturingLog()
			server := watchedServer(echoChannel(nil), newWatch(verifier, nil), sessionCaller(), authkit.SessionCredential("sid"))
			cli := dialWatched(server)

			Expect(closeCode(cli)).To(Equal(int(wskit.CloseCredentialInvalid)))
			Eventually(func() []map[string]any { return log.typed(wskit.LogTypeCredentialInvalid) }).Should(HaveLen(1))
			Expect(log.typed(wskit.LogTypeCredentialInvalid)[0]).To(HaveKeyWithValue("path", testPath))
		})

		It("ends the connection with 4403 once the caller no longer meets the channel's conditions", func() {
			verifier := &answers{}
			verifier.answer(sessionCaller("things:read"), nil)
			log := capturingLog()
			module := echoChannel(func(c *wskit.Channel) {
				c.Security = &wskit.SecuritySpec{Scopes: []string{"things:read"}}
			})
			server := watchedServer(module, newWatch(verifier, nil), sessionCaller("things:read"), authkit.SessionCredential("sid"))
			cli := dialWatched(server)
			stillOpen(cli)

			verifier.answer(sessionCaller(), nil)

			Expect(closeCode(cli)).To(Equal(int(wskit.CloseForbidden)))
			Eventually(func() []map[string]any { return log.typed(wskit.LogTypeConnectionForbidden) }).Should(HaveLen(1))
		})

		// An outage says nothing about anybody's credential. Ending every
		// connection because the store blinked would disconnect everyone at
		// once — and they would all reconnect into the same outage.
		It("keeps the connection while the store cannot answer, reporting it, and asks again", func() {
			verifier := &answers{}
			verifier.answer(nil, authkit.ErrCredentialUnavailable)
			log := capturingLog()
			server := watchedServer(echoChannel(nil), newWatch(verifier, nil), sessionCaller(), authkit.SessionCredential("sid"))
			cli := dialWatched(server)

			Eventually(verifier.asked).Should(BeNumerically(">=", 3))

			stillOpen(cli)
			Expect(log.typed(wskit.LogTypeRecheckUnavailable)).NotTo(BeEmpty())
			Expect(log.typed(wskit.LogTypeCredentialInvalid)).To(BeEmpty())
		})

		It("stops asking once the connection is closed", func() {
			verifier := &answers{}
			verifier.answer(sessionCaller(), nil)
			server := watchedServer(echoChannel(nil), newWatch(verifier, nil), sessionCaller(), authkit.SessionCredential("sid"))
			cli := dialWatched(server)
			Eventually(verifier.asked).Should(BeNumerically(">=", 1))

			Expect(cli.Close()).To(Succeed())
			time.Sleep(3 * recheckInterval)
			settled := verifier.asked()

			Consistently(verifier.asked, 5*recheckInterval).Should(Equal(settled))
		})
	})

	Describe("the caller a connection's handlers see", func() {
		// writeChannel adds test.write, which needs things:write, next to the
		// open echo.
		writeChannel := func(onClose func(*authkit.Principal)) *wskit.Module {
			return echoChannel(func(c *wskit.Channel) {
				c.Receive = append(c.Receive, wskit.Receive[echo]("test.write", wskit.ReceiveOpts{
					Security: &wskit.MessageSecurity{Scopes: []string{"things:write"}},
				}, func(ctx context.Context, event *wsevent.Message, msg *echo) error {
					return reply.Send(ctx, event.Conn, msg)
				}))
				c.Configure = func(m *wsconfig.Module) {
					m.Heartbeat.Disable = true
					m.OnClose = func(ctx context.Context, _ *wsevent.Close) error {
						if onClose != nil {
							caller, _ := authkit.PrincipalFrom(ctx)
							onClose(caller)
						}
						return nil
					}
				}
			})
		}

		// writeAnswer sends test.write and reports the envelope type that came
		// back: the reply, or an error.
		writeAnswer := func(cli *websocket.Conn) func() string {
			return func() string {
				send(cli, `{"type":"test.write","data":{"message":"hi"}}`)
				return nextEnvelope(cli).Type
			}
		}

		It("is the one the latest recheck found, so a message is judged by what the caller holds now", func() {
			verifier := &answers{}
			verifier.answer(sessionCaller(), nil)
			server := watchedServer(writeChannel(nil), newWatch(verifier, nil), sessionCaller(), authkit.SessionCredential("sid"))
			cli := dialWatched(server)
			Expect(writeAnswer(cli)()).To(Equal(wskit.ErrorMessageName))

			verifier.answer(sessionCaller("things:write"), nil)

			Eventually(writeAnswer(cli)).Should(Equal("test.echo_reply"))
		})

		// A rechecked caller is rebuilt by the store, which knows nothing of
		// the application's policy. Forgetting to attach it again would refuse
		// a caller holding admin quietly.
		It("counts the application's scope policy after a recheck too", func() {
			expander, err := authkit.NewScopeImplications(map[string][]string{"admin": {"things:write"}})
			Expect(err).NotTo(HaveOccurred())
			verifier := &answers{}
			verifier.answer(sessionCaller(), nil)
			server := watchedServer(writeChannel(nil), newWatch(verifier, expander), sessionCaller(), authkit.SessionCredential("sid"))
			cli := dialWatched(server)

			verifier.answer(sessionCaller("admin"), nil)

			Eventually(writeAnswer(cli)).Should(Equal("test.echo_reply"))
		})

		It("is the latest one in OnClose as well", func() {
			closedAs := make(chan *authkit.Principal, 1)
			verifier := &answers{}
			verifier.answer(sessionCaller("things:read"), nil)
			server := watchedServer(writeChannel(func(p *authkit.Principal) { closedAs <- p }),
				newWatch(verifier, nil), sessionCaller(), authkit.SessionCredential("sid"))
			cli := dialWatched(server)
			Eventually(verifier.asked).Should(BeNumerically(">=", 1))

			Expect(cli.Close()).To(Succeed())

			var caller *authkit.Principal
			Eventually(closedAs).Should(Receive(&caller))
			Expect(caller.Scopes).To(Equal([]string{"things:read"}))
		})
	})

	It("follows nothing for an anonymous connection", func() {
		verifier := &answers{}
		server := watchedServer(echoChannel(nil), newWatch(verifier, nil), nil, nil)
		cli := dialWatched(server)

		time.Sleep(4 * recheckInterval)

		stillOpen(cli)
		Expect(verifier.asked()).To(BeZero())
	})

	// "Raw" describes the framing, not the access: a hand-built module's
	// connection must not outlive its credential either.
	It("watches a raw channel the same way", func() {
		raw := wsconfig.NewDefaultModule(testPath)
		raw.Heartbeat.Disable = true
		server := watchedServer(wskit.Raw(raw, nil), newWatch(&answers{}, nil),
			&authkit.Principal{Subject: "usr_1", Scheme: authkit.SchemeJWT},
			authkit.TokenCredential(time.Now().Add(200*time.Millisecond)))
		cli := dialWatched(server)

		Expect(closeCode(cli)).To(Equal(int(wskit.CloseCredentialInvalid)))
	})

	// Wiring mistakes that would otherwise leave a connection open for as long
	// as it likes. They are refused at open rather than served unwatched.
	Describe("a connection the watch cannot follow", func() {
		It("is refused when its caller came with no credential", func() {
			server := watchedServer(echoChannel(nil), newWatch(&answers{}, nil), sessionCaller(), nil)
			cli := dialWatched(server)

			Expect(closeCode(cli)).NotTo(Equal(int(websocket.CloseNormalClosure)))
		})

		It("is refused when Track did not run before the upgrade", func() {
			watch := newWatch(&answers{}, nil)
			module := echoChannel(nil)
			rm := module.RuntimeModule()
			rm.HTTPMiddlewares = append([]rest.Handler{verifiedAs(sessionCaller(), authkit.SessionCredential("sid"))}, rm.HTTPMiddlewares...)
			watch.Install(rm, module.Security())
			mux := http.NewServeMux()
			mux.HandleFunc(rm.Path, wsserver.New(rm).Handler())
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)
			cli := dialWatched(server)

			Expect(closeCode(cli)).NotTo(Equal(int(websocket.CloseNormalClosure)))
		})
	})
})
