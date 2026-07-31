// Package auth wires credential verification into the application.
//
// The verification itself lives in plamo/authkit; this package only reads the
// configuration and hands the verifier to the HTTP pipeline and the WebSocket
// router through dependency injection.
package auth

import (
	"fmt"
	"slices"

	"github.com/ensoria/config/pkg/appconfig"

	"github.com/ensoria/config/pkg/env"
	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// defaultModule is the configuration module the auth settings are read from.
const defaultModule = "default"

// The credentials this template ships with so that a checkout boots and its
// endpoints can be called without setting anything up.
//
// Both are published with the template: a token signed with DevSecret can be
// forged by anyone, and DevAPIKey is printed in the repository. Outside local
// and test the application refuses to start with either — a convenience default
// must not be able to become a production credential by being left alone.
const (
	DevSecret = "ensoria-local-development-secret-change-me"
	DevAPIKey = "ensoria-local-development-api-key-change-me"
)

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
		if err := checkDevCredentials(*envVal, params.Auth); err != nil {
			return nil, err
		}
		return authkit.NewVerifier(params.Auth, nil)
	}
}

// checkDevCredentials refuses the credentials shipped with the template outside
// the environments they exist for.
//
// Each credential is only judged where it is actually used. A value that
// verifies nothing cannot protect anything either, and refusing to start over
// one would report a danger that does not exist — with advice the deployment
// has already taken.
func checkDevCredentials(envVal string, auth *appconfig.Auth) error {
	if auth == nil || devCredentialsAllowed(envVal) {
		return nil
	}
	// Secret signs and verifies tokens only in hs256 mode. Under jwks, or with
	// no mode at all, it is read by nothing.
	if auth.Mode == appconfig.AuthModeHS256 && auth.Secret == DevSecret {
		return fmt.Errorf("auth: AUTH_SECRET is still the secret shipped with the template, "+
			"which is public and cannot protect the %s environment: "+
			"set AUTH_SECRET to a secret of your own, or use AUTH_MODE=jwks", envVal)
	}
	if slices.Contains(auth.APIKeys, DevAPIKey) {
		return fmt.Errorf("auth: AUTH_API_KEYS still contains the key shipped with the template, "+
			"which is public and cannot protect the %s environment: "+
			"issue keys of your own", envVal)
	}
	return nil
}

// devCredentialsAllowed reports whether the shipped credentials may be used
// as-is. Only the environments a developer runs on their own machine qualify.
func devCredentialsAllowed(envVal string) bool {
	switch env.Environment(envVal) {
	case env.Local, env.Test:
		return true
	default:
		return false
	}
}
