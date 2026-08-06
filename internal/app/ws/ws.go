package ws

import (
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsrouter"
	"go.uber.org/fx"
)

// CreateWSRouter collects the WebSocket modules and puts the credential check in
// front of every upgrade.
//
// Guarding here rather than in each module is deliberate: a module added later
// cannot forget the guard, which would otherwise leave that endpoint reachable
// without authentication.
func CreateWSRouter(modules []*wskit.Module, verifier authkit.Verifier) *wsrouter.Router {
	guard := middleware.AuthUpgrade(verifier)
	runtime := make([]*wsconfig.Module, 0, len(modules))
	for _, m := range modules {
		rm := m.RuntimeModule()
		rm.AddHTTPMiddleware(guard)
		runtime = append(runtime, rm)
	}

	return &wsrouter.Router{
		Modules: runtime,
	}
}

// InjectWSModules tags the first parameter as the WebSocket module group. The
// remaining parameters (the credential verifier) are resolved by type.
func InjectWSModules(f any) any {
	return fx.Annotate(f, fx.ParamTags(dikit.GroupTagWSModules, ``))
}
