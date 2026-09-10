// This file holds the application's scope policy: which scope stands for which
// others. It is application code, not framework code, and it is meant to be
// edited — the framework resolves the table and enforces the result, and takes
// no view on what admin ought to mean here.
//
// It lives in Go rather than in the configuration on purpose. A table of
// implications is an authorization policy, not a per-environment setting: it
// goes through the compiler and through review, and it changes in the same
// commit and the same deployment as the endpoints whose scopes it talks about.
// Putting it in the environment would make "admin means something different in
// staging" a thing that can happen by accident.

package auth

import "github.com/ensoria/ensoria-template/internal/plamo/authkit"

// scopeImplications states which scope stands for which others.
//
// Only the left-hand side is a scope anyone is issued; the right-hand side is
// what an endpoint declares. So a caller holding admin alone satisfies an
// endpoint asking for orders:write, without admin appearing in that
// endpoint's declaration.
//
// Chains work: listing "manager" here and giving manager its own row would let
// admin reach everything manager reaches. The closure is resolved at startup,
// and a cycle refuses to start.
//
// ⚠ Deliberately absent: "orders:write" implying "orders:read". It is a
// perfectly reasonable policy — whoever may change a thing may usually read it
// — and it is left out because the template uses the difference to teach
// something else. POST /order/payment-callback is called with an API key
// holding orders:write alone, and the README shows that same key being refused
// by GET /order. Adding the implication here would make that example stop
// demonstrating anything. Add it in a real project if it is what you mean.
//
// An empty table is valid and means no expansion at all, which is what a
// deployment whose identity provider already issues the full set of scopes
// wants.
var scopeImplications = map[string][]string{
	"admin": {"orders:read", "orders:write", "users:read", "users:write"},
}

// ScopeImplicationTable returns the policy as written.
//
// The generated documentation reads it: an endpoint declaring orders:write says
// nothing about admin reaching it, so a document built from the declarations
// alone would leave a reader who holds admin unable to tell what they may call.
//
// A copy is returned so that a reader cannot edit the policy by editing what it
// was shown.
func ScopeImplicationTable() map[string][]string {
	out := make(map[string][]string, len(scopeImplications))
	for scope, implied := range scopeImplications {
		out[scope] = append([]string(nil), implied...)
	}
	return out
}

// NewScopeExpander resolves the policy above into the form the request path
// uses.
//
// A cycle in the table fails here, at startup, rather than on the request that
// happens to walk it. Nothing about a cycle depends on which request arrives,
// so there is no reason to find out later than this.
func NewScopeExpander() (authkit.ScopeExpander, error) {
	return authkit.NewScopeImplications(scopeImplications)
}
