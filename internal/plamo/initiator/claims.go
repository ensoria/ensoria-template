package initiator

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/loggear/pkg/loggear"
)

// forbiddenClaims are the names a ClaimSet refuses, and a received Initiator
// may not carry.
//
//   - scope: an Initiator is never an input to authorization, and carrying
//     scopes is how a consumer would be tempted to authorize on data it never
//     verified. Authorize at the entry point, with the Principal.
//   - the claims RFC 7519 registers: sub is carried as the ID already, and the
//     rest (iss, aud, exp, nbf, iat, jti) describe the original token, which
//     means nothing once carried somewhere else — an exp would look like a
//     deadline the Initiator has, and it has none.
var forbiddenClaims = map[string]string{
	"scope": "scopes stay at the entry point; authorize there, with the Principal",
	"iss":   "it describes the original token, not who started the work",
	"sub":   "it is carried as the Initiator's ID already",
	"aud":   "it describes the original token, not who started the work",
	"exp":   "it describes the original token; an Initiator has no expiry",
	"nbf":   "it describes the original token; an Initiator has no expiry",
	"iat":   "it describes the original token, not who started the work",
	"jti":   "it describes the original token, not who started the work",
}

// MaxReportedDroppedClaims is how many distinct dropped claim names a ClaimSet
// reports. The names come from other producers, which this application does not
// control, so neither the memory that remembers them nor the log may grow
// without bound. Past the limit one last record says that reporting stopped.
const MaxReportedDroppedClaims = 32

// The labels on the records this package writes. They are constants so that a
// log platform can be given a search that survives a change of wording.
//
// ⚠ They are separate types on purpose. A dropped claim is ordinary — another
// service carries more than this one does — and is reported once per name, at
// Info. A malformed value means a producer broke the convention, and is a
// warning every time.
const (
	// LogTypeClaimDropped records a claim that arrived on a message or job but
	// is not in this application's ClaimSet, so it was dropped.
	LogTypeClaimDropped = "initiator_claim_dropped_log"
	// LogTypeHeaderMalformed records a wire form that could not be read, so the
	// value was restored as unknown.
	LogTypeHeaderMalformed = "initiator_header_malformed_log"
	// LogTypeClaimsUnencodable records claims that could not be written as
	// JSON, so they were left off an outgoing message.
	LogTypeClaimsUnencodable = "initiator_claims_unencodable_log"
)

// ClaimSet is the set of claim names the application carries in an Initiator:
// the ones read off a caller at an entry point, the ones kept from a message or
// job that arrived, and so the ones sent on. One list, in all three places, so
// that what a handler reads is the same however the work reached it.
//
// It is application policy, written in Go and reviewed (see the template's
// internal/app/auth), not configuration: which claims leave this service is a
// question about personal data, and "staging carries the email address" must
// not be a typo away.
//
// A nil ClaimSet is the empty one: it carries no claims.
type ClaimSet struct {
	names   []string
	allowed map[string]struct{}

	// The names already reported as dropped. A ClaimSet is built once per
	// process, so this is "once per process" without a package-level variable.
	mu        sync.Mutex
	reported  map[string]struct{}
	saturated bool
}

// NewClaimSet builds the set from names. It refuses scope and the RFC 7519
// registered claims (see the comment on forbiddenClaims for why each), and an
// empty name; the error names the offending claim. A name listed twice is
// listed once.
func NewClaimSet(names []string) (*ClaimSet, error) {
	set := &ClaimSet{allowed: make(map[string]struct{}, len(names)), reported: make(map[string]struct{})}
	var errs []error
	for _, name := range names {
		if name == "" {
			errs = append(errs, errors.New("initiator: an empty claim name cannot be carried"))
			continue
		}
		if reason, forbidden := forbiddenClaims[name]; forbidden {
			errs = append(errs, fmt.Errorf("initiator: claim %q cannot be carried in an Initiator: %s", name, reason))
			continue
		}
		set.allowed[name] = struct{}{}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	for name := range set.allowed {
		set.names = append(set.names, name)
	}
	slices.Sort(set.names)
	return set, nil
}

// Names lists the claims carried, sorted. The generated documentation reads it
// to say which attributes a message's ce_authclaims may hold.
func (s *ClaimSet) Names() []string {
	if s == nil {
		return nil
	}
	return slices.Clone(s.names)
}

// carries reports whether name is in the set.
func (s *ClaimSet) carries(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s.allowed[name]
	return ok
}

// pick keeps the claims in the set that claims has. A claim in the set that the
// caller does not have is simply absent. Nil when nothing is kept.
func (s *ClaimSet) pick(claims authkit.Claims) authkit.Claims {
	var kept authkit.Claims
	for name, value := range claims {
		if !s.carries(name) {
			continue
		}
		if kept == nil {
			kept = make(authkit.Claims)
		}
		kept[name] = value
	}
	return kept
}

// receive keeps the claims in the set from what arrived on a message or job,
// and reports each name it drops (see reportDropped).
func (s *ClaimSet) receive(claims map[string]any, source string) authkit.Claims {
	kept := s.pick(claims)
	for name := range claims {
		if !s.carries(name) {
			s.reportDropped(name, source)
		}
	}
	return kept
}

// reportDropped records, once per name, that a claim arrived and was dropped,
// and how to keep it. Only the name and where it arrived are written, never the
// value: it may well be the personal data this list exists to keep out.
//
// A nil set has nowhere to remember what it reported, so it reports nothing.
func (s *ClaimSet) reportDropped(name, source string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, done := s.reported[name]; done || s.saturated {
		return
	}
	if len(s.reported) >= MaxReportedDroppedClaims {
		s.saturated = true
		loggear.Info("more claims arrived that this application does not carry; further names are not reported",
			"type", LogTypeClaimDropped,
			"source", source,
			"reported", len(s.reported))
		return
	}
	s.reported[name] = struct{}{}
	loggear.Info("a claim arrived that this application does not carry in an Initiator, so it was dropped; "+
		"to read it, add it to the Initiator claim list (internal/app/auth)",
		"type", LogTypeClaimDropped,
		"claim", name,
		"source", source)
}
