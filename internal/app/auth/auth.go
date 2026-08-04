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
	"github.com/ensoria/ensoria-template/internal/plamo/stub"
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

// TODO: 最終的なテンプレートとしては調整する
// DevPaymentAPIKey stands for a caller that may do one thing and no more.
//
// POST /order/payment-callback declares Schemes: [apiKey] and Scopes:
// [orders:write], and the point of that declaration is lost if the only API key
// in the project can do everything. This key holds orders:write alone, so the
// difference between "authenticated" and "permitted" can actually be observed:
// it opens the callback and is refused by GET /order.
//
// Unlike DevAPIKey it is not in the configuration. It exists only inside the
// development key store, which cannot be built outside local and test.
const DevPaymentAPIKey = "ensoria-local-development-payment-provider-key"

// The callers the development key store hands back. Names rather than ids,
// because the only place they appear is a local log.
const (
	DevSubject        = "local-dev"
	DevPaymentSubject = "payment-provider"
)

// TODO: 最終的なテンプレートとしては調整する
// 全体でスコープをそもそも、`orders`や`users`のようなリソース単位で分け内容にするか?
// devScopes returns every permission the template's endpoints declare, which is
// what the configured keys are given locally so that any endpoint can be tried.
//
// It is a function rather than a package-level slice so that a caller cannot
// append to the one copy everyone reads.
func devScopes() []string {
	return []string{"orders:read", "orders:write", "users:read", "users:write"}
}

// NewVerifier builds the verifier described by the configuration for the given
// environment.
//
// A configuration that cannot be satisfied (a shared-secret setup with no
// secret, an unknown mode) fails here, at startup, rather than on the first
// request that presents a token.
//
// Applications that keep API keys outside the configuration replace this
// constructor with one that passes their own authkit.KeyStore. The development
// store below is where that argument goes; swapping it is a one-line change.
func NewVerifier(envVal *string) func() (authkit.Verifier, error) {
	return func() (authkit.Verifier, error) {
		params, err := registry.ModuleParams(defaultModule)
		if err != nil {
			return nil, fmt.Errorf("auth: reading the %s configuration: %w", defaultModule, err)
		}
		if err := checkDevCredentials(*envVal, params.Auth); err != nil {
			return nil, err
		}
		keys, err := devKeyStore(*envVal, params.Auth)
		if err != nil {
			return nil, err
		}
		return authkit.NewVerifier(params.Auth, keys)
	}
}

// devKeyStore gives the configured API keys permissions they cannot carry on
// their own, and adds one key that deliberately has fewer.
//
// It returns nil outside local and test, which leaves authkit with the keys
// from the configuration — the same behaviour as before this existed. A
// deployment therefore gains nothing from this function, and loses nothing to
// it either.
//
// nil is also returned when the configuration lists no API keys at all: an
// application that turned them off must not have them turned back on here, and
// one that set AUTH_API_KEYS_EXTERNAL is expected to inject a store of its own,
// which this would shadow.
func devKeyStore(envVal string, cfg *appconfig.Auth) (authkit.KeyStore, error) {
	if !devCredentialsAllowed(envVal) || cfg == nil || len(cfg.APIKeys) == 0 {
		return nil, nil
	}

	// The keys still come from the configuration, so adding one to
	// AUTH_API_KEYS locally keeps working. Only the permissions are added here.
	keys := make(map[string]*authkit.Principal, len(cfg.APIKeys)+1)
	for _, key := range cfg.APIKeys {
		keys[key] = &authkit.Principal{Subject: DevSubject, Scopes: devScopes()}
	}
	// TODO: 最終的なテンプレートとしては調整する
	keys[DevPaymentAPIKey] = &authkit.Principal{
		Subject: DevPaymentSubject,
		Scopes:  []string{"orders:write"},
	}
	return stub.NewAPIKeyStore(envVal, keys)
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
