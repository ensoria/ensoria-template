package restkit

import (
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
	if !principal.HasScheme(schemes) || !principal.HasScopes(scopes) {
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
		for _, ctrl := range []rest.Controller{m.Get, m.Post, m.Put, m.Patch, m.Delete} {
			doc, ok := ctrl.(Documented)
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
