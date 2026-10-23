package initiator

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/ensoria/loggear/pkg/loggear"
)

// The headers an Initiator travels in. The names are the CloudEvents Kafka
// binding's names for the authcontext attributes, and the same on every broker
// (they also satisfy SQS / SNS attribute naming).
//
// Only the framework writes them: the template's publisher refuses metadata
// that names one, and its subscriber removes them from the metadata handed to a
// handler, so the one way to read them is From.
const (
	// HeaderKind is the kind (CloudEvents authtype). Always written.
	HeaderKind = "ce_authtype"
	// HeaderID is the ID (CloudEvents authid). Written when there is one.
	HeaderID = "ce_authid"
	// HeaderClaims is the claims as a JSON object in plain UTF-8 (CloudEvents
	// authclaims). Written when there are some.
	HeaderClaims = "ce_authclaims"
)

// ReservedHeaders lists the header names the wire form uses.
func ReservedHeaders() []string {
	return []string{HeaderKind, HeaderID, HeaderClaims}
}

// IsReservedHeader reports whether key is one of the wire form's headers.
func IsReservedHeader(key string) bool {
	return slices.Contains(ReservedHeaders(), key)
}

// Headers writes the value in its wire form.
//
// The kind is always written, unknown included: a consumer can then tell "the
// producer lost track of who started this" (unknown) from "the producer does not
// follow this convention" (no header at all). The ID and the claims are written
// only when there are some.
//
// Claims that cannot be written as JSON are left off, with an error logged,
// rather than failing the publish: the value is information, and losing part of
// it must not stop the work it describes.
func (i *Initiator) Headers() map[string]string {
	headers := map[string]string{HeaderKind: string(i.Kind())}
	if id := i.ID(); id != "" {
		headers[HeaderID] = id
	}
	if claims := i.Claims(); len(claims) > 0 {
		encoded, err := encodeClaims(claims)
		if err != nil {
			loggear.Error("the Initiator's claims could not be written as JSON and were left off",
				"type", LogTypeClaimsUnencodable,
				"error", err)
		} else {
			headers[HeaderClaims] = encoded
		}
	}
	return headers
}

// encodeClaims writes claims as a JSON object, without escaping <, > and & (the
// value is a header, not HTML, and plain UTF-8 is the convention).
func encodeClaims(claims map[string]any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(claims); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// FromHeaders restores an Initiator from its wire form, read out of a message's
// or job's metadata. source names where it arrived (a channel, a job name) for
// the logs.
//
// It never refuses: an Initiator is context, not a gate.
//
//   - No header at all (a producer that does not follow the convention) reads
//     as KindUnknown, quietly.
//   - A malformed value reads as KindUnknown, with a warning
//     (LogTypeHeaderMalformed): an ID or claims without a kind, claims that are
//     not a JSON object, or claims naming scope or an RFC 7519 registered claim
//     (the rules of ClaimSet hold on the receiving side too). The warning names
//     what was wrong, never the values.
//   - A kind this package does not define is taken as it is: CloudEvents lets
//     implementations add values.
//   - Claims not in claims are dropped, quietly but for one record per name
//     (LogTypeClaimDropped): the list says what this application carries, in
//     and out alike.
func FromHeaders(headers map[string]string, claims *ClaimSet, source string) *Initiator {
	kind := headers[HeaderKind]
	id := headers[HeaderID]
	rawClaims, hasClaims := headers[HeaderClaims]

	if kind == "" {
		if id == "" && !hasClaims {
			return unknown()
		}
		return malformed(source, "an ID or claims without a kind")
	}

	var received map[string]any
	if hasClaims {
		decoded, reason := decodeClaims(rawClaims)
		if reason != "" {
			return malformed(source, reason)
		}
		received = decoded
	}

	return &Initiator{kind: Kind(kind), id: id, claims: claims.receive(received, source)}
}

// decodeClaims reads the claims header. reason is non-empty when it cannot be
// accepted.
func decodeClaims(raw string) (map[string]any, string) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, "claims are not JSON"
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, "claims are not a JSON object"
	}
	for name := range object {
		if _, forbidden := forbiddenClaims[name]; forbidden {
			return nil, "claims name " + name + ", which an Initiator never carries"
		}
	}
	return object, ""
}

// malformed restores an unreadable value as unknown, and says why.
func malformed(source, reason string) *Initiator {
	loggear.Warn("a message carried an Initiator that could not be read; it was restored as unknown",
		"type", LogTypeHeaderMalformed,
		"source", source,
		"reason", reason)
	return unknown()
}
