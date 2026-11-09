package wskit

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsconn"
	"github.com/ensoria/websocket/pkg/wsevent"
	"github.com/ensoria/websocket/pkg/wssession"
)

// The close codes a connection is ended with when its caller's credential
// stops holding. They are HTTP's 401 and 403 moved into the 4000–4999 range
// the protocol leaves to applications, and they tell a client the same two
// things.
const (
	// CloseCredentialInvalid means the credential the connection was opened
	// with is no longer valid: a token expired, a session was signed out or
	// ended, a key was removed. Authenticate again before reconnecting.
	CloseCredentialInvalid wsconn.CloseCode = 4401
	// CloseForbidden means the caller is still who they were but no longer
	// meets the channel's conditions. Reconnecting is refused the same way.
	CloseForbidden wsconn.CloseCode = 4403
)

// The reasons sent with those codes. They say what happened and nothing
// about the caller.
const (
	reasonCredentialInvalid = "the credential is no longer valid"
	reasonForbidden         = "the caller no longer meets the channel's conditions"
)

// The labels on the records the watch writes.
//
// ⚠ An ended credential and an unreachable store are separate types. The first
// is ordinary — a token expired, somebody signed out — and nobody should be
// paged for it; the second is an outage. Sharing a type would make every alert
// depend on getting a filter right.
const (
	// LogTypeCredentialInvalid labels a connection ended with
	// CloseCredentialInvalid.
	LogTypeCredentialInvalid = "ws_credential_invalid_log"
	// LogTypeConnectionForbidden labels a connection ended with CloseForbidden.
	LogTypeConnectionForbidden = "ws_connection_forbidden_log"
	// LogTypeRecheckUnavailable labels a recheck the credential's store did
	// not answer. The connection is kept, and asked about again next time.
	LogTypeRecheckUnavailable = "ws_recheck_unavailable_log"
)

// recheckTimeout bounds one question to the store, so that a store that hangs
// delays the next recheck instead of stopping them.
const recheckTimeout = 10 * time.Second

// jitterDivisor sets how much of the interval a wait may be shortened by: up
// to a fifth.
const jitterDivisor = 5

// CallerWatch keeps live connections from outliving the credential they were
// opened with, and keeps the caller every handler sees current.
//
// A connection is authenticated once, at the handshake, and may then stay open
// for hours. Without this, a caller who signs out, whose key is removed, or
// whose token expires keeps a working connection for as long as it lasts:
//
//   - A token is ended at its exp, with CloseCredentialInvalid. Nothing can
//     end one earlier, so nobody is asked.
//   - An API key or a session is asked about again every interval
//     (authkit.Verifier.Recheck). Gone, it is ended with
//     CloseCredentialInvalid. Still there, it replaces the connection's
//     caller — with the application's scope policy attached again — and the
//     connection is ended with CloseForbidden if that caller no longer meets
//     the channel's declaration.
//   - A store that does not answer ends nothing: it is reported, and asked
//     again next time. An outage says nothing about anybody's credential.
//   - An anonymous connection has nothing to follow.
//
// Which of the first two applies is decided by the kind of credential, never
// by the caller's claims: a session keeps the claims of the token it was traded
// for, exp included, and that exp is not the session's.
//
// The caller a handler reads (authkit.PrincipalFrom, initiator.From) is the
// one the latest recheck found, for every OnMessage, OnClose and OnError —
// including a message's own declaration (MessageSecurity), which is judged
// against it.
//
// It takes two calls to put in front of a module, and both are needed: Track
// among the pre-upgrade handlers, after the authentication and the channel's
// check, and Install on the module. A connection that reaches the watch
// without Track having seen it is refused rather than served unwatched.
type CallerWatch struct {
	verifier authkit.Verifier
	scopes   authkit.ScopeExpander
	interval time.Duration
}

// NewCallerWatch builds a watch that asks verifier about API keys and sessions
// every interval, and attaches scopes — the application's scope policy — to
// whoever it finds. A nil scopes attaches none.
func NewCallerWatch(verifier authkit.Verifier, scopes authkit.ScopeExpander, interval time.Duration) (*CallerWatch, error) {
	if verifier == nil {
		return nil, errors.New("wskit: a caller watch needs a verifier to recheck with")
	}
	if interval <= 0 {
		return nil, errors.New("wskit: the recheck interval must be positive")
	}
	return &CallerWatch{verifier: verifier, scopes: scopes, interval: interval}, nil
}

// liveCaller is one connection's caller, as current as the last recheck made
// it, and the credential it is rechecked with. It is created before the
// upgrade and reached through the connection's context, which every phase of
// the connection derives from.
type liveCaller struct {
	credential *authkit.Credential
	caller     atomic.Pointer[authkit.Principal]
}

// liveCallerKey is unexported so that nothing outside this package can put a
// caller of its own in front of the watch.
type liveCallerKey struct{}

// liveCallerFrom returns the connection's live caller, or nil for an anonymous
// connection or one Track did not see.
func liveCallerFrom(ctx context.Context) *liveCaller {
	live, _ := ctx.Value(liveCallerKey{}).(*liveCaller)
	return live
}

// Track is the pre-upgrade handler that starts following the caller the
// handshake was verified as. It never refuses: an anonymous handshake has
// nothing to follow, and it lets it through as it is.
func (w *CallerWatch) Track() rest.Handler {
	return func(r *rest.Request) *rest.Response {
		caller, ok := authkit.PrincipalFrom(r.Context())
		if !ok {
			return nil
		}
		credential, _ := authkit.CredentialFrom(r.Context())
		live := &liveCaller{credential: credential}
		live.caller.Store(caller)
		r.SetContext(context.WithValue(r.Context(), liveCallerKey{}, live))
		return nil
	}
}

// Install puts the watch on a module, outermost in each chain, so that the
// module's own middlewares already see the current caller: OnOpen starts
// following the connection, and OnMessage, OnClose and OnError put the
// current caller on their context. security is the channel's declaration,
// which a rechecked caller has to go on meeting.
func (w *CallerWatch) Install(m *wsconfig.Module, security *SecuritySpec) {
	m.OnOpenMiddlewares = append([]wsconfig.OnOpenMiddleware{w.follow(security)}, m.OnOpenMiddlewares...)
	m.OnMessageMiddlewares = append([]wsconfig.OnMessageMiddleware{currentOnMessage}, m.OnMessageMiddlewares...)
	m.OnCloseMiddlewares = append([]wsconfig.OnCloseMiddleware{currentOnClose}, m.OnCloseMiddlewares...)
	m.OnErrorMiddlewares = append([]wsconfig.OnErrorMiddleware{currentOnError}, m.OnErrorMiddlewares...)
}

// follow starts watching a connection once it has opened. The watching stops
// with the connection's context, which ends when the connection does.
func (w *CallerWatch) follow(security *SecuritySpec) wsconfig.OnOpenMiddleware {
	return func(next wsconfig.OnOpenHandler) wsconfig.OnOpenHandler {
		return func(ctx context.Context, event *wsevent.Open) error {
			live := liveCallerFrom(ctx)
			if _, verified := authkit.PrincipalFrom(ctx); verified {
				// Either would leave the connection open for as long as it
				// likes, so the connection is refused instead.
				if live == nil {
					return errors.New("wskit: the connection's caller was not tracked before the upgrade: " +
						"put CallerWatch.Track among the pre-upgrade handlers, after the authentication")
				}
				if live.credential == nil {
					return errors.New("wskit: the connection's caller came with no credential to recheck: " +
						"the verifier has to set VerifyResult.Credential whenever it sets Principal")
				}
			}

			if err := next(ctx, event); err != nil {
				return err
			}
			if live != nil {
				go w.watch(ctx, event.Session, live, security)
			}
			return nil
		}
	}
}

// watch ends the connection when its credential stops holding. It returns
// when it has ended the connection, or when ctx ends with the connection.
func (w *CallerWatch) watch(ctx context.Context, session *wssession.Session, live *liveCaller, security *SecuritySpec) {
	if expires, ok := live.credential.Expires(); ok {
		w.endAtExpiry(ctx, session, live, expires)
		return
	}
	for {
		timer := time.NewTimer(w.nextWait())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if !w.recheck(ctx, session, live, security) {
			return
		}
	}
}

// endAtExpiry ends a token's connection when the token expires.
func (w *CallerWatch) endAtExpiry(ctx context.Context, session *wssession.Session, live *liveCaller, expires time.Time) {
	timer := time.NewTimer(time.Until(expires))
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		end(session, live, CloseCredentialInvalid)
	}
}

// recheck asks about the credential once and acts on the answer. It reports
// whether the connection is still to be watched.
func (w *CallerWatch) recheck(ctx context.Context, session *wssession.Session, live *liveCaller, security *SecuritySpec) bool {
	askCtx, cancel := context.WithTimeout(ctx, recheckTimeout)
	defer cancel()
	principal, err := w.verifier.Recheck(askCtx, live.credential)
	if ctx.Err() != nil {
		// The connection went away while the store was being asked, which is
		// what made the question fail, if it did.
		return false
	}

	switch {
	case err == nil && principal == nil:
		// Not an answer the verifier may give, and not one to read as
		// "anonymous": the connection was opened by somebody.
		end(session, live, CloseCredentialInvalid)
		return false
	case err == nil:
		caller := principal.WithScopeExpander(w.scopes)
		live.caller.Store(caller)
		switch security.Decide(caller) {
		case Unauthenticated:
			end(session, live, CloseCredentialInvalid)
			return false
		case Forbidden:
			end(session, live, CloseForbidden)
			return false
		default:
			return true
		}
	case errors.Is(err, authkit.ErrCredentialUnavailable):
		loggear.Warn("the store behind a WebSocket connection's credential could not be asked about it; the connection is kept",
			slog.String("type", LogTypeRecheckUnavailable),
			slog.String("path", pathOf(session)),
			slog.String("scheme", live.credential.Scheme()),
			slog.String("error", err.Error()),
		)
		return true
	default:
		end(session, live, CloseCredentialInvalid)
		return false
	}
}

// end closes the connection with code and records why.
func end(session *wssession.Session, live *liveCaller, code wsconn.CloseCode) {
	logType, reason := LogTypeCredentialInvalid, reasonCredentialInvalid
	if code == CloseForbidden {
		logType, reason = LogTypeConnectionForbidden, reasonForbidden
	}
	if !session.Terminate(code, reason) {
		// It was already closing, for a reason of its own.
		return
	}
	loggear.Info("a WebSocket connection was ended because its caller's credential no longer holds",
		slog.String("type", logType),
		slog.String("path", pathOf(session)),
		slog.String("scheme", live.credential.Scheme()),
		slog.Int("close_code", int(code)),
	)
}

// pathOf is the path the connection was opened on, for a record.
func pathOf(session *wssession.Session) string {
	if handshake := session.Handshake(); handshake != nil {
		return handshake.Path()
	}
	return ""
}

// nextWait is the interval shortened by up to a fifth, at random, so that
// connections opened together — every client reconnecting after a deploy — do
// not go on asking the store together. It is never longer than the interval,
// so a revocation still reaches every connection within one.
func (w *CallerWatch) nextWait() time.Duration {
	spread := int64(w.interval / jitterDivisor)
	if spread <= 0 {
		return w.interval
	}
	return w.interval - time.Duration(rand.Int64N(spread))
}

// current puts the connection's current caller on ctx.
func current(ctx context.Context) context.Context {
	if live := liveCallerFrom(ctx); live != nil {
		return authkit.WithPrincipal(ctx, live.caller.Load())
	}
	return ctx
}

func currentOnMessage(next wsconfig.OnMessageHandler) wsconfig.OnMessageHandler {
	return func(ctx context.Context, event *wsevent.Message) error {
		return next(current(ctx), event)
	}
}

func currentOnClose(next wsconfig.OnCloseHandler) wsconfig.OnCloseHandler {
	return func(ctx context.Context, event *wsevent.Close) error {
		return next(current(ctx), event)
	}
}

func currentOnError(next wsconfig.OnErrorHandler) wsconfig.OnErrorHandler {
	return func(ctx context.Context, event *wsevent.Error) error {
		return next(current(ctx), event)
	}
}
