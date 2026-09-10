package authkit

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ScopeExpander turns the scopes a credential carries into every scope they
// imply.
//
// It is the seam an application injects its own authorization policy through,
// and injecting nothing is a real choice: with no expander the behaviour is
// exactly what it was before this existed, which is what a deployment whose
// identity provider already hands out the full set of scopes wants.
//
// ⚠ Expansion applies to what a caller *holds*, never to what an endpoint
// *requires*. An endpoint declares the concrete permissions it needs and says
// nothing about who else might reach it; that a broader scope does is a fact
// about the caller, and it is decided here. Writing "admin or orders:write"
// into a declaration would put the same policy in every endpoint that admin is
// meant to reach.
type ScopeExpander interface {
	// Expand returns held together with every scope held implies.
	//
	// It must not modify held, and the same input must produce the same order
	// on every call: the result is read by the generated documentation, which
	// has to be identical between runs.
	Expand(held []string) []string
}

// ScopeImplications is the expander built from a table of "this scope implies
// these scopes", which is how an application states that admin stands for the
// permissions it stands for.
//
// The chain is resolved transitively — admin → manager → orders:read reaches
// orders:read — because the alternative is copying the right-hand side into
// every scope above it, and a copy that is not updated leaves "admin without
// what manager may do" as a silent inconsistency rather than a failure.
//
// The whole closure is resolved once, in the constructor, so a chain of any
// length costs a caller exactly what a single step costs. That is also where a
// cycle is refused: a → b → a says the two names mean the same permission, and
// a permission with two names is a mistake to fix rather than a shape to
// support.
type ScopeImplications struct {
	// closure maps a scope to everything it implies, directly or through a
	// chain, in a sorted order. Scopes that imply nothing are absent.
	closure map[string][]string
}

// Compile-time proof that the table can be used where the seam is declared.
var _ ScopeExpander = (*ScopeImplications)(nil)

// resolution states used while walking the table. A scope being walked when it
// is reached again is what a cycle looks like.
const (
	unvisited int8 = iota
	visiting
	resolved
)

// NewScopeImplications resolves a table of implications into an expander.
//
// The table is normalised on the way in: a scope implying itself adds nothing
// and is dropped rather than reported as a cycle, and a right-hand side listing
// the same scope twice is folded. Both are ways of writing a table that means
// what it says, and neither is worth failing a startup over.
//
// An empty or nil table produces an expander that expands nothing, which is
// valid: an application may declare the policy and start out with it empty.
func NewScopeImplications(table map[string][]string) (*ScopeImplications, error) {
	direct, err := normalize(table)
	if err != nil {
		return nil, err
	}

	s := &ScopeImplications{closure: make(map[string][]string, len(direct))}
	state := make(map[string]int8, len(direct))
	// Sorted, so that a table with more than one cycle names the same one on
	// every run. A startup failure that reports a different scope each time is
	// far harder to act on than one that is boring.
	for _, scope := range slices.Sorted(maps.Keys(direct)) {
		if _, err := resolve(scope, direct, s.closure, state, nil); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// normalize checks the table and removes what carries no meaning.
func normalize(table map[string][]string) (map[string][]string, error) {
	direct := make(map[string][]string, len(table))
	for scope, implied := range table {
		if scope == "" {
			return nil, errors.New("authkit: a scope implication has no scope on its left-hand side")
		}
		// Seeding with the scope itself is what drops a self-implication.
		seen := map[string]bool{scope: true}
		kept := make([]string, 0, len(implied))
		for _, name := range implied {
			if name == "" {
				return nil, fmt.Errorf("authkit: scope %q implies an empty scope name", scope)
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			kept = append(kept, name)
		}
		direct[scope] = kept
	}
	return direct, nil
}

// resolve returns everything one scope implies, memoising as it goes.
//
// path carries the scopes currently being walked, so that a cycle is reported
// as the route that closes it (admin -> manager -> admin) rather than as the
// bare fact that one exists. Which entry to delete is the question the person
// reading the startup failure actually has.
func resolve(
	scope string,
	direct map[string][]string,
	closure map[string][]string,
	state map[string]int8,
	path []string,
) ([]string, error) {
	switch state[scope] {
	case resolved:
		return closure[scope], nil
	case visiting:
		return nil, fmt.Errorf(
			"authkit: the scope implications contain a cycle (%s): "+
				"two names for one permission are better written as one scope",
			strings.Join(append(slices.Clone(path), scope), " -> "))
	}

	state[scope] = visiting
	path = append(slices.Clone(path), scope)

	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, name := range direct[scope] {
		add(name)
		implied, err := resolve(name, direct, closure, state, path)
		if err != nil {
			return nil, err
		}
		for _, deeper := range implied {
			add(deeper)
		}
	}

	slices.Sort(out)
	closure[scope] = out
	state[scope] = resolved
	return out, nil
}

// Expand returns held followed by everything held implies.
//
// The caller's own scopes come first and in the order they arrived, so that a
// person reading EffectiveScopes can tell what the credential carried from what
// the policy added. Both halves are deterministic, which is what the interface
// promises.
func (s *ScopeImplications) Expand(held []string) []string {
	if s == nil || len(held) == 0 {
		return held
	}

	seen := make(map[string]bool, len(held))
	for _, scope := range held {
		seen[scope] = true
	}

	// Cloned rather than appended to: held belongs to the Principal, and a
	// slice with spare capacity would let this write into it.
	out := slices.Clone(held)
	for _, scope := range held {
		for _, implied := range s.closure[scope] {
			if seen[implied] {
				continue
			}
			seen[implied] = true
			out = append(out, implied)
		}
	}
	return out
}

// Closure returns the resolved table: each scope that implies something,
// against everything it implies.
//
// It exists so that the policy can be looked at rather than only obeyed — a
// test asserting the chain came out right, a startup log, and the generated
// documentation, which has to tell a reader that admin reaches an endpoint
// declaring orders:write. Scopes that imply nothing are left out; they are
// rows that would say nothing.
//
// The result is a copy, because the caller is usually a renderer and a renderer
// that sorts its input must not be able to sort the policy.
func (s *ScopeImplications) Closure() map[string][]string {
	if s == nil {
		return nil
	}
	out := make(map[string][]string, len(s.closure))
	for scope, implied := range s.closure {
		if len(implied) == 0 {
			continue
		}
		out[scope] = slices.Clone(implied)
	}
	return out
}
