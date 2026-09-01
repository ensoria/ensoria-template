// Package stub holds stand-ins for the parts an application would otherwise
// need real infrastructure for, so that a checkout can be run and exercised
// before any of it exists.
//
// Everything here refuses to be constructed outside the local and test
// environments. A stub is only safe because it cannot reach production, and
// "the package is called stub" is an expectation rather than a guarantee — the
// guarantee has to be in the code.
//
// These are meant to be read and thrown away. A project that reaches the point
// of issuing real API keys replaces APIKeyStore with one backed by its own
// storage; the interface it satisfies (authkit.KeyStore) is the same either way.
package stub

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// APIKeyStore answers API key lookups from a fixed table.
//
// It exists because the keys listed in the configuration cannot carry
// permissions: authkit's built-in store returns a caller with no scopes, so an
// endpoint declaring any scope refuses every configured key with 403. This one
// gives each key a caller of its own, which is what a real store backed by a
// database would do.
type APIKeyStore struct {
	principals map[string]*authkit.Principal
}

// NewAPIKeyStore builds a store from a table of key to caller.
//
// It refuses every environment but local and test. The keys handed to it are
// written down somewhere — in source, in a checked-in configuration — and a
// credential that is written down is a credential everyone has.
//
// The callers are copied, so the table the argument came from cannot change
// what the store answers afterwards. Scheme is filled in rather than asked for:
// a caller who arrives with an API key authenticated with an API key, and there
// is nothing for the argument to decide.
func NewAPIKeyStore(envVal string, keys map[string]*authkit.Principal) (*APIKeyStore, error) {
	if !allowedIn(envVal) {
		return nil, fmt.Errorf("stub: the API key stub only runs in the %s and %s environments "+
			"(got %q): its keys are written down, so they cannot protect anything else — "+
			"implement authkit.KeyStore against your own storage instead",
			env.Local, env.Test, envVal)
	}
	if len(keys) == 0 {
		return nil, errors.New("stub: the API key stub was given no keys, so it would refuse every caller")
	}

	principals := make(map[string]*authkit.Principal, len(keys))
	for key, principal := range keys {
		if key == "" {
			return nil, errors.New("stub: an API key cannot be empty")
		}
		if principal == nil {
			return nil, fmt.Errorf("stub: API key %q has no caller behind it", key)
		}
		// Without a subject nothing identifies the caller in a log, which is the
		// one thing this store exists to provide over the configured keys.
		if principal.Subject == "" {
			return nil, fmt.Errorf("stub: API key %q has a caller with no subject", key)
		}
		principals[key] = &authkit.Principal{
			Subject: principal.Subject,
			Scopes:  slices.Clone(principal.Scopes),
			Scheme:  authkit.SchemeAPIKey,
			Claims:  maps.Clone(principal.Claims),
		}
	}
	return &APIKeyStore{principals: principals}, nil
}

// Lookup returns the caller the key belongs to.
//
// The returned caller is a copy, so application code that adds a scope to the
// principal it was handed does not widen what the next request may do.
func (s *APIKeyStore) Lookup(_ context.Context, key string) (*authkit.Principal, error) {
	principal, ok := s.principals[key]
	if !ok {
		// The key itself is never quoted back: it would end up in a log, and an
		// unknown key is as likely to be a real one sent to the wrong place.
		return nil, authkit.ErrKeyNotFound
	}
	return &authkit.Principal{
		Subject: principal.Subject,
		Scopes:  slices.Clone(principal.Scopes),
		Scheme:  principal.Scheme,
		Claims:  maps.Clone(principal.Claims),
	}, nil
}

// allowedIn reports whether stubs may be built for the given environment.
// Only the environments a developer runs on their own machine qualify.
func allowedIn(envVal string) bool {
	switch env.Environment(envVal) {
	case env.Local, env.Test:
		return true
	default:
		return false
	}
}
