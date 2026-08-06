// Package mbkit provides the typed declarations for the message broker surface:
// Subscription[Msg] for what the application consumes and Publication[Msg] for
// what it emits.
//
// It is the mb counterpart of restkit. Its purpose is the same: lift the facts a
// document generator needs — the message type, its validation rules, the
// subscribe/publish options, the behaviour — out of imperative wiring code and
// onto declared fields, where msgdoc can reflect over them.
//
// The declarations are not documentation-only. A subscription's handler is
// reached through its decoding adapter, and a publication is the only way the
// application publishes, so the described contract and the running code cannot
// drift apart: changing one without the other stops compiling.
package mbkit

import "github.com/ensoria/mb/pkg/mb"

// Metadata is the broker metadata carried alongside a message (topic, partition,
// offset, key, and whatever the publisher attached).
type Metadata map[string]string

// BehaviorSpec declares the facts about an operation that no type can express.
//
// Declare "none" explicitly rather than leaving a field empty, so a reader can
// tell "nothing happens" from "nobody wrote it down" — the generated document
// renders the first as a fact and the second as TODO.
type BehaviorSpec struct {
	// SideEffects lists what handling or emitting the message changes.
	SideEffects []string
	// Idempotent states whether handling the same message twice is safe.
	//
	// nil means undeclared. It matters more here than over HTTP: an
	// at-least-once broker redelivers on its own schedule, so a handler that is
	// not idempotent is a defect waiting for a redelivery, not a caller's
	// problem.
	Idempotent *bool
	// Preconditions lists what must already be true for handling to succeed.
	Preconditions []string
	// Scopes are the permissions required to use this channel.
	Scopes []string
	// Delivery describes the delivery semantics the code relies on.
	Delivery DeliverySpec
	// Ordering states the order guarantee relied upon ("none",
	// "per-partition by user id").
	Ordering string
}

// DeliverySpec is the declared half of the delivery story. The other half — what
// the SubscribeOption / PublishOption values actually resolve to — is read back
// from the broker config, so the document can show a promise next to the
// configuration meant to keep it.
type DeliverySpec struct {
	// Guarantee is the delivery semantics relied upon ("at-least-once").
	Guarantee string
	// Notes are further declared facts (retry policy, dead-letter destination).
	Notes []string
}

// ResolveSubscribeConfig applies the declared options to the broker defaults and
// returns the configuration that will really be in force.
//
// Reading the options back rather than documenting the declaration is what keeps
// the delivery section honest: the options are opaque functions, and the only
// way to learn what WithErrorStrategy(...) did is to run it.
func ResolveSubscribeConfig(opts []mb.SubscribeOption) *mb.SubscribeConfig {
	cfg := mb.DefaultSubscribeConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// ResolvePublishConfig is ResolveSubscribeConfig for the publishing side.
func ResolvePublishConfig(opts []mb.PublishOption) *mb.PublishConfig {
	cfg := mb.DefaultPublishConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
