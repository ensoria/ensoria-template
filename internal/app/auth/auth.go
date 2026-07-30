// Package auth wires credential verification into the application.
//
// The verification itself lives in plamo/authkit; this package only reads the
// configuration and hands the verifier to the HTTP pipeline and the WebSocket
// router through dependency injection.
package auth

import (
	"fmt"

	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// defaultModule is the configuration module the auth settings are read from.
const defaultModule = "default"

// DevSecret is the shared secret this template ships with so that a checkout
// boots and its admin endpoints can be called without setting anything up.
//
// It is published with the template, so a token signed with it can be forged by
// anyone. Outside local and test it is treated as "no secret was configured"
// and the application refuses to start — a convenience default must not be able
// to become a production credential by being left alone.
const DevSecret = "ensoria-local-development-secret-change-me"

// NewVerifier builds the verifier described by the configuration for the given
// environment.
//
// A configuration that cannot be satisfied (a shared-secret setup with no
// secret, an unknown mode) fails here, at startup, rather than on the first
// request that presents a token.
//
// Applications that keep API keys outside the configuration replace this
// constructor with one that passes their own authkit.KeyStore.
func NewVerifier(envVal *string) func() (authkit.Verifier, error) {
	return func() (authkit.Verifier, error) {
		params, err := registry.ModuleParams(defaultModule)
		if err != nil {
			return nil, fmt.Errorf("auth: reading the %s configuration: %w", defaultModule, err)
		}
		if err := checkDevSecret(*envVal, params.Auth.Secret); err != nil {
			return nil, err
		}
		return authkit.NewVerifier(params.Auth, nil)
	}
}

// checkDevSecret refuses the secret shipped with the template outside the
// environments it exists for.
func checkDevSecret(envVal, secret string) error {
	if secret != DevSecret || devSecretAllowed(envVal) {
		return nil
	}
	return fmt.Errorf("auth: AUTH_SECRET is still the secret shipped with the template, "+
		"which is public and cannot protect the %s environment: "+
		"set AUTH_SECRET to a secret of your own, or use AUTH_MODE=jwks", envVal)
}

// devSecretAllowed reports whether the shipped secret may be used as-is.
// Only the environments a developer runs on their own machine qualify.
func devSecretAllowed(envVal string) bool {
	switch env.Environment(envVal) {
	case env.Local, env.Test:
		return true
	default:
		return false
	}
}
