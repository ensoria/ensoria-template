// Package auth wires credential verification into the application.
//
// The verification itself lives in plamo/authkit; this package only reads the
// configuration and hands the verifier to the HTTP pipeline and the WebSocket
// router through dependency injection.
package auth

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// defaultModule is the configuration module the auth settings are read from.
const defaultModule = "default"

// NewVerifier builds the verifier described by the configuration.
//
// A configuration that cannot be satisfied (a shared-secret setup with no
// secret, an unknown mode) fails here, at startup, rather than on the first
// request that presents a token.
//
// Applications that keep API keys outside the configuration replace this
// constructor with one that passes their own authkit.KeyStore.
func NewVerifier() (authkit.Verifier, error) {
	params, err := registry.ModuleParams(defaultModule)
	if err != nil {
		return nil, fmt.Errorf("auth: reading the %s configuration: %w", defaultModule, err)
	}
	return authkit.NewVerifier(params.Auth, nil)
}
