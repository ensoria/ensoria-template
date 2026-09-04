package ws

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsrouter"
	"go.uber.org/fx"
)

// defaultModule is the configuration module the trusted origins are read from.
// They belong to the application rather than to one channel.
const defaultModule = "default"

// CreateWSRouter collects the WebSocket modules and puts two checks in front of
// every upgrade: where the handshake came from, and who is making it.
//
// Guarding here rather than in each module is deliberate: a module added later
// cannot forget the guards, which would otherwise leave that endpoint reachable
// without them.
//
// The trusted origins are taken as an argument rather than read here, so that
// the check can be exercised against an origin list a test chose.
//
// ⚠ The origin check runs first, and it has to. A handshake from a page on
// another site is refused before the session store is asked about the cookie
// the browser attached to it — and refusing it is not optional the way it is
// for an ordinary GET: the same-origin policy does not apply to WebSocket
// connections, so a page anywhere can open one and read everything it carries.
// See middleware.UpgradeOrigin.
func CreateWSRouter(modules []*wskit.Module, verifier authkit.Verifier, origins *middleware.Origins) *wsrouter.Router {
	guards := []rest.Handler{
		middleware.UpgradeOrigin(origins),
		middleware.AuthUpgrade(verifier),
	}

	runtime := make([]*wsconfig.Module, 0, len(modules))
	for _, m := range modules {
		rm := m.RuntimeModule()
		rm.AddHTTPMiddlewares(guards...)
		runtime = append(runtime, rm)
	}

	return &wsrouter.Router{
		Modules: runtime,
	}
}

// NewTrustedOrigins reads which origins this deployment calls its own frontend.
//
// It is in the dependency graph rather than read where it is used so that the
// answer is resolved once, by one reading of CORS_ALLOW_ORIGIN, and handed to
// everything that needs it — the CORS middleware, the cross-origin check, and
// the upgrade guard above. They disagree about who the frontend is only if they
// are allowed to work it out separately.
func NewTrustedOrigins() (*middleware.Origins, error) {
	params, err := registry.ModuleParams(defaultModule)
	if err != nil {
		return nil, fmt.Errorf("ws: reading the %s configuration: %w", defaultModule, err)
	}
	return middleware.ParseOrigins(params.CORS.AllowOrigin()), nil
}

// InjectWSModules tags the first parameter as the WebSocket module group. The
// remaining parameters (the credential verifier, the CORS settings) are
// resolved by type.
func InjectWSModules(f any) any {
	return fx.Annotate(f, fx.ParamTags(dikit.GroupTagWSModules, ``))
}
