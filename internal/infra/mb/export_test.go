package mb

import (
	"github.com/ensoria/config/pkg/appconfig"
	enmb "github.com/ensoria/mb/pkg/mb"
)

// Test-only bridge. This file is compiled into the test binary only, so nothing
// here widens the package's public API.
//
// The tests live in package mb_test, which cannot reach the assembly step on
// its own. Exposing it is what lets the assembly rules be checked against
// parameters built by hand, with no registry state involved.
var BrokerConfigFromParams func(*appconfig.Parameters) *enmb.Config = brokerConfigFromParams
