package initiator

import (
	"context"
	"maps"
)

// Propagator carries who started the work through a string map that travels
// with it — the metadata a job queue stores beside a job, for one.
//
// It has the Inject / Extract pair the worker library's propagators have, so a
// worker can be handed one as it is: the worker calls Inject inside Enqueue and
// Extract right before the job's handler runs, and the handler reads the value
// with From, as code does everywhere else.
//
// The map holds the same wire form a message's headers do (Headers,
// FromHeaders), with the same rules: the claims kept are the ones in the
// ClaimSet, and a malformed value is restored as unknown, never refused.
type Propagator struct {
	claims *ClaimSet
	source string
}

// NewPropagator builds a propagator keeping the claims in claims. source names
// the path in the records FromHeaders writes ("worker job").
func NewPropagator(claims *ClaimSet, source string) *Propagator {
	return &Propagator{claims: claims, source: source}
}

// Inject writes who started the work ctx belongs to into metadata.
func (p *Propagator) Inject(ctx context.Context, metadata map[string]string) {
	maps.Copy(metadata, From(ctx).Headers())
}

// Extract returns ctx carrying the value metadata holds. metadata may be nil
// (work stored before anything was written), which reads as unknown.
func (p *Propagator) Extract(ctx context.Context, metadata map[string]string) context.Context {
	return With(ctx, FromHeaders(metadata, p.claims, p.source))
}
