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

// Publication is the typed declaration of one publication (a SEND operation),
// and at the same time the only way the application publishes it.
//
// Publishing goes through the declaration rather than around it because, in
// pub/sub, the contract belongs to the publisher: a consumer in another team or
// another service has nothing else to read. A raw mb.Publish call would emit a
// message no document knows about, which is exactly the drift this exists to
// prevent.
//
// Declare it as a literal and bind it with NewPublication, which supplies the
// broker connection:
//
//	func NewUserCreatedPublication(publish mb.Publish) *mbkit.Publication[dto.UserCreated] {
//	    return mbkit.NewPublication(publish, &mbkit.Publication[dto.UserCreated]{
//	        Target:  "user.created",
//	        Summary: "Announce that a user was created",
//	    })
//	}
type Publication[Msg any] struct {
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

	// publish is the broker-facing function. It is unexported so a Publication
	// can only become usable through NewPublication: a declaration built as a
	// bare literal would otherwise compile, inject and then fail at the first
	// publish, in production.
	publish mb.Publish
}

// NewPublication binds a declaration to the broker and returns the publication
// the application injects.
//
// The declaration is copied, so the returned publication is frozen at
// construction and cannot be changed afterwards through the literal that
// produced it.
func NewPublication[Msg any](publish mb.Publish, decl *Publication[Msg]) *Publication[Msg] {
	bound := *decl
	bound.publish = publish
	return &bound
}

// Publish validates the message, encodes it and sends it to the declared target.
//
// meta may be nil. opts are applied after the declared Options, so a single call
// can override a declared default without changing what every other call does.
func (p *Publication[Msg]) Publish(ctx context.Context, msg *Msg, meta Metadata, opts ...mb.PublishOption) error {
	if p.publish == nil {
		return fmt.Errorf("mbkit: publication %q was not bound to a broker (use mbkit.NewPublication)", p.Target)
	}
	if errs := vkit.Object(msg, p.BodyRules...); errs.HasErrors() {
		return fmt.Errorf("mbkit: publishing to %s: %w", p.Target, invalidMessageError{target: p.Target, errs: errs})
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mbkit: encoding the message for %s: %w", p.Target, err)
	}
	return p.publish(ctx, p.Target, data, meta, append(p.Options, opts...)...)
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
	return &PublicationDoc{
		Target:      p.Target,
		Summary:     p.Summary,
		Description: p.Description,
		FieldDocs:   p.FieldDocs,
		Task:        p.Task,
		AlsoRead:    p.AlsoRead,
		Related:     p.Related,
		MsgType:     reflect.TypeFor[Msg](),
		BodyRules:   p.BodyRules,
		Behavior:    p.Behavior,
		Config:      ResolvePublishConfig(p.Options),
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
