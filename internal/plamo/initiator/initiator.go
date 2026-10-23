// Package initiator answers "whose action started this work?" the same way on
// every path the work can take: an HTTP request, a WebSocket message, a message
// broker subscription, a worker job, a scheduled task.
//
// It is not authorization. authkit.Principal is the caller this process
// verified, and only an entry point (HTTP, WebSocket) has one; it is what
// authorization reads. An Initiator is who started the work, carried along to
// places where nothing was verified — a consumer on the far side of a broker
// reads one that a producer wrote. The rule is:
//
//	authorize with the Principal; ask the Initiator who did it.
//
// That is why an Initiator holds no scopes, and why the claims it may carry
// are a fixed list the application states (ClaimSet), with scope and the
// token's own registered claims refused outright.
//
// # Where the value comes from
//
// Read it with From, which always answers:
//
//   - At an entry point the authentication middleware marks the request
//     (WithEntry), and the value is derived from the Principal when it is read,
//     so it always agrees with the caller currently on the context. No caller
//     reads as KindUnauthenticated.
//   - A scheduled task is marked as the system (WithSystem). The system is a
//     label, not a principal: it holds no permission of any kind.
//   - A message or job carries the value it was published with (FromHeaders,
//     With).
//   - A context nobody marked reads as KindUnknown — work that lost track of
//     its origin, or came in a way this package does not cover (gRPC).
//
// When a context is marked more than once, the mark put on last is the one
// read, as context values always are.
//
// # The wire form
//
// Across a broker the value travels as three headers named after the
// CloudEvents authcontext extension (HeaderKind, HeaderID, HeaderClaims). The
// messages themselves are not CloudEvents; only the attribute names and their
// meaning are shared. A value restored from headers is a fact as of the moment
// the message was published, not a credential: nobody re-checks it, and it
// must never be the basis for granting anything.
package initiator

import (
	"context"
	"maps"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// Kind says what kind of party started the work. The values are the CloudEvents
// authcontext authtype values, verbatim.
type Kind string

const (
	// KindAppUser is a caller authenticated by the application: a session, or a
	// JWT — any JWT, including one a service obtained with client credentials.
	KindAppUser Kind = "app_user"
	// KindUser is a person's account on the infrastructure the work runs on —
	// an AWS IAM user, a Google Cloud user account — as opposed to KindAppUser,
	// a person this application authenticates. An operator changing something
	// through the cloud console or CLI is one. This application never writes it:
	// its authentication knows no such accounts. It arrives from producers that
	// are not Ensoria, which write the CloudEvents authtype of their own.
	KindUser Kind = "user"
	// KindServiceAccount is a non-human account on that infrastructure — a
	// Google Cloud service account, a Kubernetes ServiceAccount, an assumed AWS
	// IAM role — such as a batch job running under its own identity. Like
	// KindUser, this application never writes it, and receives it from other
	// producers.
	KindServiceAccount Kind = "service_account"
	// KindAPIKey is a caller authenticated with an API key.
	KindAPIKey Kind = "api_key"
	// KindSystem is work the application started on its own (a scheduled task).
	KindSystem Kind = "system"
	// KindUnauthenticated is a request that carried no credential.
	KindUnauthenticated Kind = "unauthenticated"
	// KindUnknown is work whose origin is not known.
	KindUnknown Kind = "unknown"
)

// Initiator is who started a piece of work. Read it with From.
//
// It is immutable: the accessors hand out copies, so code reading one cannot
// change what the next reader sees.
type Initiator struct {
	kind   Kind
	id     string
	claims authkit.Claims
}

// Kind is what kind of party started the work.
func (i *Initiator) Kind() Kind {
	if i == nil || i.kind == "" {
		return KindUnknown
	}
	return i.kind
}

// ID identifies who started it, when the kind has an identity: the Principal's
// Subject at an entry point, unchanged so that a consumer can look the party up
// in its own data. Empty for the system, an unauthenticated request and unknown
// work.
//
// ⚠ It is the sub claim as the identity provider wrote it. An identity provider
// that puts an email address there makes this personal data, carried wherever
// the work goes.
func (i *Initiator) ID() string {
	if i == nil {
		return ""
	}
	return i.id
}

// Claims are the claims carried with the Initiator, limited to the
// application's ClaimSet. Nil when there are none. Read them with the typed
// accessors (String, Int, ...): a value that crossed a broker has JSON's types.
//
// They are information, never permission. Decide what a party may do from your
// own data, keyed by ID — not from what another service wrote here.
func (i *Initiator) Claims() authkit.Claims {
	if i == nil || len(i.claims) == 0 {
		return nil
	}
	return maps.Clone(i.claims)
}

// unknown is the value of work whose origin is not known.
func unknown() *Initiator {
	return &Initiator{kind: KindUnknown}
}

// derive reads the Initiator off a verified caller.
func derive(caller *authkit.Principal, claims *ClaimSet) *Initiator {
	return &Initiator{
		kind:   kindOf(caller.Scheme),
		id:     caller.Subject,
		claims: claims.pick(caller.Claims),
	}
}

// kindOf maps how a caller authenticated to what kind of party it is. A scheme
// authkit does not define has no honest label, and reads as unknown (its ID is
// still kept).
func kindOf(scheme string) Kind {
	switch scheme {
	case authkit.SchemeJWT, authkit.SchemeSession:
		return KindAppUser
	case authkit.SchemeAPIKey:
		return KindAPIKey
	default:
		return KindUnknown
	}
}

// contextKey is unexported so that only this package can mark a context.
type contextKey struct{}

// mark is what a context carries: either "this is an entry point, derive from
// the caller" or a value fixed when the mark was put on.
type mark struct {
	entry  bool
	claims *ClaimSet
	fixed  *Initiator
}

// WithEntry marks ctx as an entry point. From then derives the Initiator from
// the Principal on the context when it is read, carrying the claims in claims
// (nil carries none), and reads a context with no Principal as
// KindUnauthenticated.
//
// The authentication middleware calls it on every request it lets through.
func WithEntry(ctx context.Context, claims *ClaimSet) context.Context {
	return context.WithValue(ctx, contextKey{}, &mark{entry: true, claims: claims})
}

// WithSystem marks ctx as work the application started on its own.
func WithSystem(ctx context.Context) context.Context {
	return With(ctx, &Initiator{kind: KindSystem})
}

// With marks ctx with a fixed value: one restored from a message or a job. A
// nil value marks the context as unknown.
func With(ctx context.Context, initiator *Initiator) context.Context {
	if initiator == nil {
		initiator = unknown()
	}
	return context.WithValue(ctx, contextKey{}, &mark{fixed: initiator})
}

// From answers who started the work ctx belongs to. It always returns a value:
// not knowing is itself an answer, KindUnknown.
func From(ctx context.Context) *Initiator {
	m, _ := ctx.Value(contextKey{}).(*mark)
	switch {
	case m != nil && m.fixed != nil:
		return m.fixed.clone()
	case m != nil && m.entry:
		if caller, ok := authkit.PrincipalFrom(ctx); ok {
			return derive(caller, m.claims)
		}
		return &Initiator{kind: KindUnauthenticated}
	default:
		// No mark, but a verified caller leaves no doubt about who it is. With
		// no entry mark there is no claim set either, so it carries no claims.
		if caller, ok := authkit.PrincipalFrom(ctx); ok {
			return derive(caller, nil)
		}
		return unknown()
	}
}

// clone copies the value, so a context keeps its own.
func (i *Initiator) clone() *Initiator {
	return &Initiator{kind: i.kind, id: i.id, claims: maps.Clone(i.claims)}
}
