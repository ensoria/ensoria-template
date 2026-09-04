// Package api registers the session exchange endpoints with the application.
//
// It is a package of its own, imported for its init(), so that a project that
// does not authenticate browsers with a cookie removes it in one line — the
// blank import in internal/app/bootstrap/server. Nothing else refers to it.
//
// ⚠ Removing the import and leaving AUTH_SESSION_STORE set is fine: the
// application simply verifies session cookies it has no endpoint to create.
// The reverse is not, and it is refused at startup rather than served: keeping
// the endpoints while unsetting AUTH_SESSION_STORE would leave a sign-in route
// that answers 503 to everyone.
package api

import (
	"github.com/ensoria/ensoria-template/internal/app/auth/api/controller/http"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
	"github.com/ensoria/rest/pkg/rest"
)

// SessionPath is where a session is created and ended. It is one path with two
// methods rather than /login and /logout because a session is a resource: POST
// makes one, DELETE ends it, and there is nothing to name in between.
//
// It is exported so that the generated documentation can name the path this
// application actually serves rather than a copy of it (see describeScheme in
// internal/app/bootstrap/describe). A project that needs /session for something
// of its own changes this constant, and the documentation follows.
const SessionPath = "/session"

// NewSessionModule serves the session exchange at SessionPath.
//
// Both dependencies may be nil, which is what they are when the application
// does not authenticate browsers with a cookie. The module is still built —
// failing here would break document generation, which resolves every module
// with nothing behind it — and the startup checks are what refuse to serve the
// endpoints in that state.
func NewSessionModule(sessions sessionkit.Store, cookies *sessionkit.Cookies) *rest.Module {
	return &rest.Module{
		Path:   SessionPath,
		Post:   restkit.NewController(http.NewCreateSession(sessions, cookies)),
		Delete: restkit.NewController(http.NewDeleteSession(sessions, cookies)),
	}
}

func init() {
	dikit.AppendConstructors([]any{
		dikit.AsHTTPModule(NewSessionModule),
	})
}
