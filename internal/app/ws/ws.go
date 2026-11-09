package ws

import (
	"fmt"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/middleware"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/websocket/pkg/wsconfig"
	"github.com/ensoria/websocket/pkg/wsrouter"
	"go.uber.org/fx"
)

// defaultModule is the configuration module the trusted origins are read from.
// They belong to the application rather than to one channel.
const defaultModule = "default"

// CreateWSRouter collects the WebSocket modules and puts three checks in front
// of every upgrade: where the handshake came from, who is making it, and
// whether the channel admits them (the channel's wskit.SecuritySpec — nil
// requires a caller, only Public opens a channel).
//
// Guarding here rather than in each module is deliberate: a module added later
// cannot forget the guards, which would otherwise leave that endpoint reachable
// without them.
//
// ⚠ The guards go in front of the channel's own pre-upgrade middlewares, not
// after them. A middleware a channel adds (one that looks a room up, say) must
// never run for a handshake that is going to be refused, or it would act for a
// caller nobody verified.
//
// It refuses to build a router for channels whose declarations cannot be
// served as written (see checkChannelSecurity), so the mistake stops the
// process at startup rather than refusing every handshake.
//
// A connection is then kept from outliving the credential it was opened with
// (wskit.CallerWatch): a token's connection ends at its exp, and a session's or
// an API key's is asked about again every settings.RecheckInterval — ended
// when the credential is gone, or when the caller no longer meets the channel.
// Every OnMessage, OnClose and OnError sees the caller the latest recheck
// found. Raw channels are watched the same way. Nil settings means the
// configuration's defaults.
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
func CreateWSRouter(
	modules []*wskit.Module,
	verifier authkit.Verifier,
	origins *middleware.Origins,
	scopes authkit.ScopeExpander,
	claims *initiator.ClaimSet,
	settings *appconfig.AuthWebSocket,
) (*wsrouter.Router, error) {
	if err := checkChannelSecurity(modules, verifier); err != nil {
		return nil, err
	}

	interval := appconfig.DefaultWebSocketRecheckInterval
	if settings != nil {
		interval = settings.RecheckInterval
	}
	watch, err := wskit.NewCallerWatch(verifier, scopes, interval)
	if err != nil {
		return nil, err
	}

	deps := &upgradeGuardDeps{origins: origins, verifier: verifier, scopes: scopes, claims: claims, watch: watch}
	runtime := make([]*wsconfig.Module, 0, len(modules))
	for _, m := range modules {
		rm := m.RuntimeModule()
		guards := make([]rest.Handler, 0, len(upgradeGuardChain))
		for _, g := range upgradeGuardChain {
			guards = append(guards, g.Build(deps, m))
		}
		rm.HTTPMiddlewares = append(guards, rm.HTTPMiddlewares...)
		watch.Install(rm, m.Security())
		runtime = append(runtime, rm)
	}

	return &wsrouter.Router{
		Modules: runtime,
	}, nil
}

// The names the generated document gives the handlers every handshake passes
// before the upgrade.
const (
	GuardUpgradeOrigin   = "upgrade-origin"
	GuardAuthUpgrade     = "auth-upgrade"
	GuardChannelSecurity = "channel-security"
	GuardCallerWatch     = "caller-watch"
)

// upgradeGuard is one handler every handshake passes before the upgrade, with
// the name the generated document gives it.
type upgradeGuard struct {
	Name  string
	Build func(d *upgradeGuardDeps, m *wskit.Module) rest.Handler
}

// upgradeGuardDeps is what the guards are built from.
type upgradeGuardDeps struct {
	origins  *middleware.Origins
	verifier authkit.Verifier
	scopes   authkit.ScopeExpander
	claims   *initiator.ClaimSet
	watch    *wskit.CallerWatch
}

// upgradeGuardChain is what runs on every handshake, in order. The order is
// the point of keeping it in one list:
//
//   - the origin check first, so a handshake from another site is refused
//     before the session store is asked about the cookie it carried;
//   - the credential check, then the channel's declaration, which reads the
//     caller the credential check found;
//   - the caller watch last, so that only a handshake every check admitted is
//     followed for the life of its connection.
var upgradeGuardChain = []upgradeGuard{
	{
		Name:  GuardUpgradeOrigin,
		Build: func(d *upgradeGuardDeps, _ *wskit.Module) rest.Handler { return middleware.UpgradeOrigin(d.origins) },
	},
	{
		Name: GuardAuthUpgrade,
		Build: func(d *upgradeGuardDeps, _ *wskit.Module) rest.Handler {
			return middleware.AuthUpgrade(d.verifier, d.scopes, d.claims)
		},
	},
	{
		Name: GuardChannelSecurity,
		Build: func(_ *upgradeGuardDeps, m *wskit.Module) rest.Handler {
			return middleware.ChannelSecurity(m.Security())
		},
	},
	{
		Name:  GuardCallerWatch,
		Build: func(d *upgradeGuardDeps, _ *wskit.Module) rest.Handler { return d.watch.Track() },
	},
}

// UpgradeGuardNames names, in order, what runs on every handshake before the
// upgrade. The generated documentation reads it, which is why it is derived
// from the chain rather than kept beside it. A fresh slice is returned.
func UpgradeGuardNames() []string {
	names := make([]string, 0, len(upgradeGuardChain))
	for _, g := range upgradeGuardChain {
		names = append(names, g.Name)
	}
	return names
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

// NewWebSocketSettings reads how live WebSocket connections are kept from
// outliving their credentials (AUTH_WS_RECHECK_INTERVAL).
func NewWebSocketSettings() (*appconfig.AuthWebSocket, error) {
	params, err := registry.ModuleParams(defaultModule)
	if err != nil {
		return nil, fmt.Errorf("ws: reading the %s configuration: %w", defaultModule, err)
	}
	return params.Auth.WebSocket, nil
}

// InjectWSModules tags the first parameter as the WebSocket module group. The
// remaining parameters (the credential verifier, the CORS settings) are
// resolved by type.
func InjectWSModules(f any) any {
	return fx.Annotate(f, fx.ParamTags(dikit.GroupTagWSModules, ``))
}
