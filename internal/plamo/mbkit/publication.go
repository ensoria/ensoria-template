package mbkit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ensoria/ensoria-template/internal/plamo/vkit"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/validator/pkg/rule"
)

// PublicationSpec is the typed declaration of one publication (a SEND
// operation).
//
// It is declared rather than left implicit because, in pub/sub, the contract
// belongs to the publisher: a consumer in another team or another service has
// nothing else to read. A raw mb.Publish call would emit a message no document
// knows about, which is exactly the drift this exists to prevent.
//
// A spec on its own cannot publish anything. Bind it to the broker with
// NewPublication, which returns the Publication the application injects:
//
//	func NewUserCreatedPublication(publish mb.Publish) *mbkit.Publication[dto.UserCreated] {
//	    return mbkit.NewPublication(publish, &mbkit.PublicationSpec[dto.UserCreated]{
//	        Target:  "user.created",
//	        Summary: "Announce that a user was created",
//	    })
//	}
type PublicationSpec[Msg any] struct {
	// Target is the channel address to publish to ("user.created"). Required.
	Target string

	// --- Prose that cannot be derived from the types ---

	// Summary is the one-line description of the event being announced.
	Summary string
	// Description is the longer explanation.
	Description string
	// FieldDocs gives the meaning of individual payload fields, keyed by field
	// path in dot / bracket notation.
	FieldDocs map[string]string
	// Task is the intent label shown in generated indexes (1-3 words).
	Task string
	// AlsoRead lists further documents to read with this one.
	AlsoRead []string
	// Related declares operations that come before or after this one.
	Related []string

	// --- Validation ---

	// BodyRules validates the message before it is published. Publishing a
	// message that violates the contract this very declaration documents is
	// worth catching here, at the only place it can still be stopped.
	BodyRules []*rule.RuleSet

	// --- Behaviour and delivery ---

	// Behavior declares side effects, idempotency, delivery and ordering.
	Behavior BehaviorSpec
	// Options are the broker publish options applied to every Publish call.
	// Per-call options passed to Publish are applied after these, so a call can
	// override the declared default.
	Options []mb.PublishOption
}

// Publication is the runtime side of a PublicationSpec: the only way the
// application publishes the message, and the documentation msgdoc reads.
//
// Its fields are unexported and it holds no declaration of its own, so a
// declaration written as a literal cannot be injected by mistake — spelling
// &mbkit.Publication[Msg]{Target: ...} does not compile, because Target belongs
// to the spec. The two types exist for exactly that reason: an unbound
// declaration would otherwise wire up cleanly and fail at the first publish, in
// production.
//
// The spec is held, never copied or modified. Build it as a literal at
// construction and do not keep a reference to it: mbkit will not notice later
// edits to its slices and maps, so a spec mutated afterwards makes the running
// code and the generated document disagree.
type Publication[Msg any] struct {
	spec    *PublicationSpec[Msg]
	publish mb.Publish
}

// NewPublication binds a declaration to the broker.
func NewPublication[Msg any](publish mb.Publish, spec *PublicationSpec[Msg]) *Publication[Msg] {
	return &Publication[Msg]{spec: spec, publish: publish}
}

// Target is the channel this publication sends to.
func (p *Publication[Msg]) Target() string {
	if p.spec == nil {
		return ""
	}
	return p.spec.Target
}

// Publish validates the message, encodes it and sends it to the declared target.
//
// meta may be nil. opts are applied after the declared Options, so a single call
// can override a declared default without changing what every other call does.
func (p *Publication[Msg]) Publish(ctx context.Context, msg *Msg, meta Metadata, opts ...mb.PublishOption) error {
	if p.spec == nil || p.publish == nil {
		return fmt.Errorf("mbkit: publication was not built with NewPublication")
	}
	if errs := vkit.Object(msg, p.spec.BodyRules...); errs.HasErrors() {
		return fmt.Errorf("mbkit: publishing to %s: %w", p.spec.Target, invalidMessageError{target: p.spec.Target, errs: errs})
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mbkit: encoding the message for %s: %w", p.spec.Target, err)
	}
	return p.publish(ctx, p.spec.Target, data, meta, append(p.spec.Options, opts...)...)
}

// PublicationDoc is the non-generic view of a publication that msgdoc reads.
type PublicationDoc struct {
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
	Config *mb.PublishConfig
}

// DocumentedPublication is satisfied by publications that expose their
// documentation. msgdoc collects the DI group through this interface.
type DocumentedPublication interface {
	PublicationDoc() *PublicationDoc
}

// PublicationDoc exposes the declaration for msgdoc.
func (p *Publication[Msg]) PublicationDoc() *PublicationDoc {
	if p.spec == nil {
		return &PublicationDoc{MsgType: reflect.TypeFor[Msg]()}
	}
	return &PublicationDoc{
		Target:      p.spec.Target,
		Summary:     p.spec.Summary,
		Description: p.spec.Description,
		FieldDocs:   p.spec.FieldDocs,
		Task:        p.spec.Task,
		AlsoRead:    p.spec.AlsoRead,
		Related:     p.spec.Related,
		MsgType:     reflect.TypeFor[Msg](),
		BodyRules:   p.spec.BodyRules,
		Behavior:    p.spec.Behavior,
		Config:      ResolvePublishConfig(p.spec.Options),
	}
}

// AsPublicationDoc adapts a bound publication to the documentation-only view the
// describe group collects.
//
// It exists because a DI group holds one type, and *Publication[Msg] is a
// different type for every message. Registering this adapter alongside the
// publication lets the service inject the typed object it publishes through,
// while describe still sees a single uniform group:
//
//	dikit.AsMBPublication(mbkit.AsPublicationDoc[dto.UserCreated]),
func AsPublicationDoc[Msg any](p *Publication[Msg]) DocumentedPublication { return p }
