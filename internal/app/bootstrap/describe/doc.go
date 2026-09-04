// Package describe resolves the application's declarations without connecting to
// any real infrastructure, and builds the specs the document generators render.
//
// Two surfaces are described, one per file. http.go builds the HTTP API spec
// (apidoc.APISpec, rendered as OpenAPI and DocAI); messaging.go builds the
// messaging spec (msgdoc.MessagingSpec, rendered as AsyncAPI) from the broker
// subscriptions, the broker publications and the WebSocket channels. The two
// resolve their declarations the same way and share what they have in common:
// the stub list in stubs.go and the security schemes in security.go.
//
// Nothing connects and nothing runs. The infrastructure types a module can
// inject are stubbed, and the fx lifecycle is never started — so no OnStart hook
// fires, no port is bound, and no broker or database is dialed. Handlers, jobs
// and subscriptions are read as declarations, never executed.
package describe

import (
	// Imported for their init(), which registers the module constructors
	// (repository / service / controller / module) with dikit. describe takes the
	// same set the application serves — anything missed is an endpoint or a
	// channel that exists but disappears from the generated documents without a
	// word.
	//
	// They belong here rather than in http.go or messaging.go because both
	// surfaces are built out of them.
	//
	// ⚠ auth/api is the pair of internal/app/bootstrap/server's blank import of
	// the same package. A project that stops serving the session endpoints has
	// to remove both, or the generated documents keep describing two endpoints
	// the application does not serve. (security.go imports it again by name,
	// for the path the session scheme's description quotes.)
	_ "github.com/ensoria/ensoria-template/internal/app/auth/api"
	_ "github.com/ensoria/ensoria-template/internal/app/scheduler/api"
	_ "github.com/ensoria/ensoria-template/internal/app/worker/api"
	_ "github.com/ensoria/ensoria-template/internal/module"
	_ "github.com/ensoria/ensoria-template/internal/query"
)
