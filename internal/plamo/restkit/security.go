package restkit

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/rest/pkg/rest"
)

// authorize checks the request against an endpoint's security declaration.
// It returns a response when the call must be refused, and nil to let it run.
//
// A nil declaration means "a verified caller is required": an endpoint whose
// author never thought about access ends up closed. Only Public opens one.
func authorize(security *SecuritySpec, r *rest.Request) *rest.Response {
	if security != nil && security.Public {
		return nil
	}

	principal, ok := authkit.PrincipalFrom(r.Context())
	if !ok {
		// Nobody is authenticated: 401 asks the caller to identify themselves.
		return UnauthenticatedResponse()
	}

	var schemes, scopes []string
	if security != nil {
		schemes, scopes = security.Schemes, security.Scopes
	}
	if !principal.HasScheme(schemes) || !principal.SatisfiesScopes(scopes) {
		// The caller is known and still may not: repeating the credential
		// would not help, so this is 403 rather than 401.
		return ForbiddenResponse()
	}
	return nil
}

// RequiresAuthentication reports whether any endpoint in the given modules needs
// a verified caller.
//
// The application uses it at startup to catch the combination that would
// otherwise refuse every request: endpoints that need a caller, and no
// configured way to verify one.
//
// Controllers that are not typed endpoints are not counted: they never reach the
// adapter, so no declaration applies to them.
func RequiresAuthentication(modules []*rest.Module) bool {
	for _, m := range modules {
		if m == nil {
			continue
		}
		for _, route := range routesOf(m) {
			doc, ok := route.controller.(Documented)
			if !ok {
				continue
			}
			if security := doc.EndpointDoc().Security; security == nil || !security.Public {
				return true
			}
		}
	}
	return false
}

// PublicResourceChecks returns the endpoints that declare a resource check on a
// public endpoint, as "GET /order" strings, in the order they were declared.
//
// The combination cannot mean anything: a resource check decides whether this
// caller may touch this resource, and a public endpoint has no caller for it to
// be about. Left alone it would be worse than meaningless — authorize() lets a
// public request past before any check runs, so the endpoint would serve
// everyone while its generated documentation described a constraint.
//
// The application refuses to start on it, which is where a contradiction
// between two declared facts belongs: nothing about it depends on which request
// arrives.
func PublicResourceChecks(modules []*rest.Module) []string {
	var conflicts []string
	for _, m := range modules {
		if m == nil {
			continue
		}
		for _, route := range routesOf(m) {
			doc, ok := route.controller.(Documented)
			if !ok {
				continue
			}
			security := doc.EndpointDoc().Security
			if security != nil && security.Public && security.Resource != nil {
				conflicts = append(conflicts, route.method+" "+m.Path)
			}
		}
	}
	return conflicts
}

// route pairs a controller with the method it answers, which a rest.Module
// keeps as separate fields. The method is part of naming an endpoint: a path on
// its own does not identify one.
type route struct {
	method     string
	controller rest.Controller
}

// routesOf lists what a module answers, in a fixed order so that a startup
// failure names the same endpoint first on every run.
func routesOf(m *rest.Module) []route {
	return []route{
		{http.MethodGet, m.Get},
		{http.MethodPost, m.Post},
		{http.MethodPut, m.Put},
		{http.MethodPatch, m.Patch},
		{http.MethodDelete, m.Delete},
	}
}

// DeclaredSchemes returns every credential kind the given modules require, in
// the order they are first declared.
//
// The application compares it against what the configuration can verify: an
// endpoint that insists on a kind of credential nothing checks would refuse
// every caller, which is a misconfiguration worth catching at startup rather
// than on the first request.
func DeclaredSchemes(modules []*rest.Module) []string {
	var schemes []string
	seen := map[string]bool{}
	for _, m := range modules {
		if m == nil {
			continue
		}
		for _, route := range routesOf(m) {
			doc, ok := route.controller.(Documented)
			if !ok {
				continue
			}
			security := doc.EndpointDoc().Security
			if security == nil || security.Public {
				continue
			}
			for _, scheme := range security.Schemes {
				if !seen[scheme] {
					seen[scheme] = true
					schemes = append(schemes, scheme)
				}
			}
		}
	}
	return schemes
}
