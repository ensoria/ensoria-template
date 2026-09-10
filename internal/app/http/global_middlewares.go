// This file declares the global middleware chain: what runs on every HTTP
// request, in order. It is separate from http.go so that adding a middleware is
// an edit to a declaration rather than to the server plumbing — everything a
// project changes to put something new in front of every request is the one
// list below, and nothing here builds a server.
//
// (Detached from the package clause on purpose: this describes the file, and a
// comment touching `package http` would become the package's documentation.)

package http

import (
	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/rest/pkg/mw"
	"github.com/ensoria/rest/pkg/rest"
)

// globalMiddlewareDeps carries what the chain is built from. It is a struct
// rather than a parameter list so that an entry below can ignore what it does
// not need without every entry restating the whole list.
type globalMiddlewareDeps struct {
	cors          *appconfig.CORS
	crossOrigin   middleware.CrossOriginChecker
	verifier      authkit.Verifier
	scopes        authkit.ScopeExpander
	panicResponse *rest.Response
}

// globalMiddleware is one link of the chain: the name it is known by outside
// this package, and how to build it once its dependencies exist.
//
// Build is a function rather than a ready middleware because the names are
// needed where the dependencies are not — the document generator resolves no
// configuration, opens no store and has no verifier, yet has to say what runs.
type globalMiddleware struct {
	Name  string
	Build func(deps *globalMiddlewareDeps) rest.Middleware
}

// globalMiddlewareChain is the single statement of what runs on every request,
// in order. The pipeline and the generated documentation are both projections
// of it — globalMiddlewares takes the Build side, GlobalMiddlewareNames the Name
// side — so a middleware added here reaches both, and one cannot describe a
// pipeline the other does not run.
//
// It was two lists until 2026-09-07, and they had already drifted: the names
// stayed at four entries after CSRF and Auth joined the chain, so every
// generated document described a pipeline the application had stopped running
// two phases earlier. Nothing could have caught it, because nothing in the type
// system relates a []rest.Middleware to a []string.
//
// # Order
//
// The chain runs outside-in, so authentication sits last: logging, panic
// recovery and CORS still apply to a request that is refused, and a CORS
// preflight (which carries no credential) is answered before authentication is
// considered.
//
// The cross-origin check sits between CORS and authentication. Before
// authentication, so a forged request is refused without the session store being
// asked about the cookie it carried; after CORS, so a preflight is still
// answered by the one middleware that knows how to answer it.
//
// ⚠ Only one of those two refuses anything. CORS tells the browser what it may
// read and refuses nothing; the cross-origin check refuses state-changing
// requests from origins this deployment does not claim. See middleware.CORS for
// why the split is that way round.
var globalMiddlewareChain = []globalMiddleware{
	{
		Name:  apidoc.MiddlewareLogging,
		Build: func(*globalMiddlewareDeps) rest.Middleware { return mw.Logging(logIncomingRequest) },
	},
	{
		Name: apidoc.MiddlewareRecovery,
		Build: func(d *globalMiddlewareDeps) rest.Middleware {
			return mw.RecoveryWithLogger(d.panicResponse, logPanicDetails)
		},
	},
	{
		Name:  apidoc.MiddlewareVerifyBodyParsable,
		Build: func(*globalMiddlewareDeps) rest.Middleware { return mw.VerifyBodyParsable },
	},
	{
		Name:  apidoc.MiddlewareCORS,
		Build: func(d *globalMiddlewareDeps) rest.Middleware { return middleware.CORS(d.cors) },
	},
	{
		Name:  apidoc.MiddlewareCSRF,
		Build: func(d *globalMiddlewareDeps) rest.Middleware { return middleware.CSRF(d.crossOrigin) },
	},
	{
		Name: apidoc.MiddlewareAuth,
		Build: func(d *globalMiddlewareDeps) rest.Middleware {
			return middleware.Auth(d.verifier, d.scopes)
		},
	},
}

// globalMiddlewares builds the chain every request passes through, in the order
// globalMiddlewareChain declares.
func globalMiddlewares(deps *globalMiddlewareDeps) []rest.Middleware {
	chain := make([]rest.Middleware, 0, len(globalMiddlewareChain))
	for _, m := range globalMiddlewareChain {
		chain = append(chain, m.Build(deps))
	}
	return chain
}

// GlobalMiddlewareNames names, in order, what the chain installs.
//
// The generated documentation reads it, and one entry in particular is acted on:
// a document describing a cookie-borne credential explains the cross-origin
// guard only when this list names it. A name missing here would understate what
// the application does, and a name outliving its middleware would promise a
// protection that is gone — which is why the names are derived from the chain
// rather than kept beside it.
//
// A fresh slice is returned because the caller hands it to a document model that
// outlives the call; sharing the backing array would let a renderer sort or
// truncate the declaration itself.
func GlobalMiddlewareNames() []string {
	names := make([]string, 0, len(globalMiddlewareChain))
	for _, m := range globalMiddlewareChain {
		names = append(names, m.Name)
	}
	return names
}
