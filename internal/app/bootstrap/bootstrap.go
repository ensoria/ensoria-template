// Package bootstrap holds the part of application startup that every entry
// point shares. Each application (server, scheduler, ...) has its own
// sub-package that composes its dependency graph; what they have in common —
// the settings that live outside that graph — lives here.
package bootstrap

import (
	"fmt"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
	"github.com/ensoria/loggear/pkg/loggear"
)

// defaultModule is the configuration module the global settings are read from.
const defaultModule = "default"

// ApplyGlobalSettings applies the settings that are not part of any dependency
// graph — the global log level and restkit's strict declaration mode — and
// reports whether fx's own construction log should be shown.
//
// Every application calls this once, after registry.InitializeConfiguration and
// before it builds its graph. Keeping it in one place is what makes the
// settings actually global: when each bootstrap applied them itself, a setting
// added to one of them was silently missing from the other, and the next
// application to be added would have missed them all. Settling them before fx
// is built also means anything logged while the graph is constructed already
// obeys the configured level.
//
// The describe commands (bootstrap/describe) deliberately do not call this.
// They read declarations without serving a request, so strict mode has nothing
// to check, and their stdout carries a JSON contract that log output must not
// share — their main fixes the log destination itself.
//
// Loading the configuration is not part of this. Its arguments belong to the
// application (which config FS it embeds) and so does the wording of the error
// it fails with. The loaded registry is taken as an argument rather than read
// from the package-level one so that a test can hand it a configuration of its
// own — the same reason infra/mb takes one.
func ApplyGlobalSettings(envVal *string, reg *registry.Registry) (outputFxLog bool, err error) {
	params, err := reg.ModuleParams(defaultModule)
	if err != nil {
		return false, fmt.Errorf("app initialization error: %w", err)
	}

	// An application that drops the HTTP app — the template exists to be
	// modified — would otherwise lose its log level without a word.
	loggear.SetLogLevel(params.Log.Level)

	// Answering with a status the endpoint never declared makes the generated
	// documentation drift from the implementation. Fail on it immediately in
	// the environments a developer works in; in production it is logged and the
	// request is served.
	restkit.SetStrictDeclarations(restkit.StrictForEnv(*envVal))

	return params.Log.Level == loggear.LogLevelDebug, nil
}
