// This file holds which claims the application carries in an Initiator — the
// record of "whose action started this work" that travels with messages and
// jobs. It is application code, meant to be edited, and it sits in Go rather
// than in the configuration for the same reason the scope policy beside it does:
// which claims leave this service is a decision about personal data, and it
// should go through review, not differ between environments by accident.

package auth

import (
	"slices"

	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
)

// initiatorClaims lists the claims this application carries in an Initiator.
//
// One list decides all three places a claim can be:
//
//   - read off the caller at an entry point (HTTP, WebSocket),
//   - kept from a message or job that arrived from another service,
//   - sent on with what this application publishes or enqueues.
//
// So a claim another service sends is visible here only when it is listed
// here. For example: service A lists tenant and email and publishes
// {"tenant":"t1","email":"x@example.com"}; this application lists tenant only.
// Its handler reads tenant, never email, and what it publishes next carries
// tenant alone. Each dropped name is logged once (type
// initiator_claim_dropped_log), which is the place to look when a claim you
// expected is missing.
//
// Refused, and the process does not start: scope (an Initiator never
// authorizes anything — authorize at the entry point, with the Principal) and
// the registered claims of RFC 7519 (iss, sub, aud, exp, nbf, iat, jti), which
// describe the original token rather than who started the work. sub travels as
// the Initiator's ID already.
//
// Empty by default: the kind and the ID (the caller's sub) travel, and nothing
// else. Add a claim only when a consumer needs it, and remember that whatever
// is listed reaches every consumer of every message this application sends.
var initiatorClaims = []string{}

// InitiatorClaimNames returns the list as written. The generated documentation
// reads it, to say which attributes a message's ce_authclaims header may hold.
//
// A copy is returned so that a reader cannot edit the list by editing what it
// was shown.
func InitiatorClaimNames() []string {
	return slices.Clone(initiatorClaims)
}

// NewInitiatorClaims resolves the list above into the form the entry points,
// the publisher and the subscribers use. A refused name fails here, at startup.
func NewInitiatorClaims() (*initiator.ClaimSet, error) {
	return initiator.NewClaimSet(initiatorClaims)
}
