package mbkit

import (
	"context"
	"reflect"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
	"github.com/ensoria/validator/pkg/verr"
)

// Subscription is the typed declaration of one subscription (a RECEIVE
// operation).
//
// The subscribe target and its options live here rather than inside a startup
// closure. That is the whole point: a target chosen imperatively at wiring time
// is invisible to reflection, so no generator can learn which channels the
// application actually consumes.
//
// Only Target and Handle are required to run. Everything else is either an
// option that changes behaviour (BodyRules, Options, OnInvalid) or read solely
// by the generators, which show TODO where a declaration is missing.
type Subscription[Msg any] struct {
	// Target is the channel address to subscribe to ("hello_world"). Required.
	Target string

	// --- Prose that cannot be derived from the types ---

	// Summary is the one-line description of why this subscription exists.
	Summary string
	// Description is the longer explanation.
	Description string
	// FieldDocs gives the meaning of individual payload fields, keyed by field
	// path in dot / bracket notation ("address.city", "items[].id").
	FieldDocs map[string]string
	// Task is the intent label shown in generated indexes (1-3 words).
	Task string
	// AlsoRead lists further documents to read with this one.
	AlsoRead []string
	// Related declares operations that come before or after this one.
	Related []string

	// --- Validation ---

	// BodyRules validates the decoded message. A violation stops the message
	// before Handle is called and is dealt with according to OnInvalid.
	BodyRules []*rule.RuleSet

	// --- Behaviour and delivery ---

	// Behavior declares side effects, idempotency, delivery and ordering.
	Behavior BehaviorSpec
	// Options are the broker subscribe options. They are applied at runtime and
	// read back for the documentation, so the delivery section describes the
	// subscription that is really running.
	Options []mb.SubscribeOption
	// OnInvalid decides what happens to a message that cannot be decoded or
	// fails BodyRules. The zero value discards it; see InvalidPolicy.
	OnInvalid InvalidPolicy

	// Handle receives the decoded, validated message. Required.
	//
	// ctx is the receive-scoped context supplied by the broker: propagate it
	// downstream so cancellation on shutdown and trace continuation both work.
	Handle func(ctx context.Context, msg *Msg, meta Metadata) error
}

// InvalidPolicy decides the fate of a message that cannot be decoded or fails
// its validation rules.
type InvalidPolicy int

const (
	// InvalidDiscard logs the failure and accepts the message so the broker does
	// not redeliver it. This is the default because a malformed message never
	// becomes valid on a retry: returning an error instead would, under the
	// default requeue strategy, put the same message back at the head of the
	// queue forever and stall every well-formed message behind it.
	InvalidDiscard InvalidPolicy = iota
	// InvalidFail returns the failure to the broker and lets the configured
	// ErrorStrategy deal with it. Choose it when the subscription has somewhere
	// safe to put the message — a dead-letter queue — and losing it silently is
	// worse than the redelivery traffic.
	InvalidFail
)

// SubscriptionDoc is the non-generic view of a subscription that msgdoc reads.
// Subscription is generic and awkward to reflect over, so the module converts it
// into this shape once.
type SubscriptionDoc struct {
	Target      string
	Summary     string
	Description string
	FieldDocs   map[string]string
	Task        string
	AlsoRead    []string
	Related     []string

	// MsgType is the declared message type, the source of the payload schema.
	MsgType reflect.Type

	BodyRules []*rule.RuleSet
	Behavior  BehaviorSpec
	// Config is what Options resolve to against the broker defaults.
	Config *mb.SubscribeConfig
}

// DocumentedSubscription is satisfied by subscription modules that expose their
// documentation. msgdoc collects the DI group through this interface.
type DocumentedSubscription interface {
	SubscriptionDoc() *SubscriptionDoc
}

// SubscriptionModule is the non-generic form of one Subscription: the handler mb
// subscribes with, and the documentation msgdoc reads, in one object.
//
// Both come from the same declaration on purpose. If the runtime handler and the
// described contract were separate registrations, one could be changed without
// the other and nothing would notice.
type SubscriptionModule struct {
	target  string
	options []mb.SubscribeOption
	doc     *SubscriptionDoc
	// receive decodes and validates the raw message, then calls the typed
	// handler. The generic type is closed over here, which is what lets this
	// struct stay non-generic and live in a single DI group.
	receive func(ctx context.Context, data []byte, meta Metadata) error
}

// NewSubscriptionModule turns a typed declaration into the module the DI group
// carries.
func NewSubscriptionModule[Msg any](sub *Subscription[Msg]) *SubscriptionModule {
	return &SubscriptionModule{
		target:  sub.Target,
		options: sub.Options,
		doc: &SubscriptionDoc{
			Target:      sub.Target,
			Summary:     sub.Summary,
			Description: sub.Description,
			FieldDocs:   sub.FieldDocs,
			Task:        sub.Task,
			AlsoRead:    sub.AlsoRead,
			Related:     sub.Related,
			MsgType:     reflect.TypeFor[Msg](),
			BodyRules:   sub.BodyRules,
			Behavior:    sub.Behavior,
			Config:      ResolveSubscribeConfig(sub.Options),
		},
		receive: func(ctx context.Context, data []byte, meta Metadata) error {
			msg, vErrs := vkit.JSONBody[Msg](data, sub.BodyRules...)
			if vErrs.HasErrors() {
				return handleInvalid(sub.Target, sub.OnInvalid, data, vErrs)
			}
			return sub.Handle(ctx, msg, meta)
		},
	}
}

// Target is the channel this module subscribes to.
func (m *SubscriptionModule) Target() string { return m.target }

// Options are the broker options this module subscribes with.
func (m *SubscriptionModule) Options() []mb.SubscribeOption { return m.options }

// SubscriptionDoc exposes the declaration for msgdoc.
func (m *SubscriptionModule) SubscriptionDoc() *SubscriptionDoc { return m.doc }

// OnReceive satisfies mb.SubscribeHandler: decode, validate, then hand the typed
// message to the declared handler.
func (m *SubscriptionModule) OnReceive(ctx context.Context, data []byte, metadata map[string]string) error {
	return m.receive(ctx, data, Metadata(metadata))
}

// handleInvalid applies the declared policy to a message that could not be
// decoded or failed validation. Either way the failure is logged: a message
// dropped without a trace is indistinguishable from one that never arrived.
func handleInvalid(target string, policy InvalidPolicy, data []byte, vErrs verr.ValidationErrors) error {
	err := invalidMessageError{target: target, errs: vErrs}
	loggear.Error("rejected an invalid message",
		"target", target,
		"policy", policy.String(),
		"reason", err.Error(),
		"data", string(data),
	)
	if policy == InvalidFail {
		return err
	}
	return nil
}

// String names the policy for logs and for the generated document.
func (p InvalidPolicy) String() string {
	switch p {
	case InvalidFail:
		return "fail"
	default:
		return "discard"
	}
}

// invalidMessageError reports a message that could not be decoded or did not
// satisfy the declared rules. It keeps the field-level errors so a dead-letter
// consumer can see which field was at fault rather than just "bad message".
type invalidMessageError struct {
	target string
	errs   verr.ValidationErrors
}

func (e invalidMessageError) Error() string {
	msg := "invalid message on " + e.target
	for _, fe := range e.errs {
		msg += "; "
		if fe.Field != "" {
			msg += fe.Field + ": "
		}
		msg += fe.Code
	}
	return msg
}

// ValidationErrors exposes the field-level failures behind this error.
func (e invalidMessageError) ValidationErrors() verr.ValidationErrors { return e.errs }
