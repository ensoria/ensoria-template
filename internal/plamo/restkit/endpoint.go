// Package restkit provides the typed HTTP endpoint (Endpoint[Req, Res]) and the
// adapter that makes it satisfy rest.Controller.
//
// Its purpose is to create a declarative surface the documentation generators can
// read: request and response types, validation rules, statuses and behaviour are
// lifted out of the imperative Handle body and onto declared fields of Endpoint.
// apidoc then reflects over those declarations to build the API spec, which
// `encli generate docai` and `encli generate openapi` render.
package restkit

import (
	"reflect"

	"github.com/ensoria/rest/pkg/rest"
	"github.com/ensoria/validator/pkg/rule"
)

// Endpoint is the typed definition of one HTTP endpoint.
//
// Req is the request body type and Res the success response body type. Pinning
// the body types as type parameters makes the compiler reject any drift between
// the declared schema and the body the handler actually returns. Endpoints
// without a body use NoBody for either parameter.
//
// # Which fields you actually have to fill in
//
// Only Handle is required to serve traffic. Every other field is marked below as
// one of:
//
//   - Required — leaving it out is a bug.
//   - Optional (runtime) — it changes how the endpoint behaves. Skipping it is a
//     real choice, not just a documentation gap.
//   - Optional (documentation only) — read solely by the generators. A project
//     that does not generate documentation can skip all of these with no effect
//     on behaviour; the generated documents simply show TODO in their place.
//   - Required when ... — normally optional, but mandatory in the stated case.
//
// So an endpoint that never generates documentation only needs Handle, plus
// whichever runtime fields it wants (validation rules, status, media type).
type Endpoint[Req any, Res any] struct {
	// --- Prose that cannot be derived from the types ---
	// Declaring it here (rather than writing it into the generated Markdown)
	// is what keeps it from being wiped out on the next generation.

	// Summary is the one-line description used as the INDEX summary and as the
	// sentence right below the endpoint heading.
	//
	// Optional (documentation only).
	Summary string
	// Description is a longer explanation of why the endpoint is called.
	//
	// Optional (documentation only).
	Description string
	// FieldDocs gives the meaning of individual body fields, keyed by the field
	// path in dot / bracket notation ("address.city", "items[].id"). Keys that
	// match no field are ignored.
	//
	// It applies to both the request and the response, so write what the field
	// means rather than what this operation does with it: a field named "name"
	// exists on both sides, and "omit to leave it unchanged" would be wrong on
	// the response. Use RequestFieldDocs / ResponseFieldDocs when the two sides
	// genuinely need different wording.
	//
	// Optional (documentation only).
	FieldDocs map[string]string
	// RequestFieldDocs and ResponseFieldDocs override FieldDocs for one side.
	// Same key format; a key present here wins over the same key in FieldDocs.
	//
	// Optional (documentation only).
	RequestFieldDocs  map[string]string
	ResponseFieldDocs map[string]string

	// Task is the client-intent label shown in the INDEX Task column (1-3 words).
	// Endpoints serving the same client task reuse the same label.
	//
	// Optional (documentation only).
	Task string
	// AlsoRead lists extra files (workflows and the like) a caller should read
	// together with this endpoint, as paths relative to the docs root.
	//
	// Optional (documentation only).
	AlsoRead []string
	// Related declares endpoints or workflows called before or after this one,
	// one free-form line each ("Fetch after creation: GET /users/{id}").
	//
	// Optional (documentation only).
	Related []string

	// IDPrefix pins the prefix used for this resource's ids in generated examples
	// ("usr"). When empty the prefix is derived from the path or field name.
	// It only affects example values, never real responses.
	//
	// Optional (documentation only).
	IDPrefix string

	// --- Validation, separated by where it applies ---
	// These do run: a violation makes the adapter answer 422 with field-level
	// errors before Handle is ever called.

	// BodyRules validates the parsed request body.
	//
	// Optional (runtime).
	BodyRules []*rule.RuleSet
	// PathRules validates path parameters, keyed by the `{name}` in the route.
	//
	// Optional (runtime).
	PathRules []*rule.RuleSet
	// QueryRules validates query parameters, keyed by the parameter name.
	//
	// Optional (runtime).
	QueryRules []*rule.RuleSet

	// --- Response ---

	// Success is the primary success status. Zero means 200.
	//
	// Optional (runtime): it decides the status a caller actually receives.
	Success int
	// ResponseHeaders documents the headers a caller should read.
	//
	// Optional (documentation only): declaring a header here does NOT send it.
	// Send the real header from Handle with rest.WithHeader.
	ResponseHeaders []HeaderSpec
	// Produces pins the response media type. Empty means content negotiation
	// (JSON by default, chosen from the Accept header).
	//
	// Optional (runtime).
	Produces string
	// Responses declares success responses other than the primary one — the
	// 200-or-201 upsert, the 202 that returns a different body, and so on.
	//
	// Required when Handle can answer with a status other than Success.
	// The adapter checks every response against these declarations, so an
	// undeclared status fails immediately in local, test and development
	// environments (and is logged in staging and production). That check is what
	// keeps the generated documentation from drifting away from the code.
	Responses []ResponseSpec

	// --- Errors ---

	// Errors declares the errors specific to this endpoint. Errors shared by the
	// whole API belong in the common conventions, not here.
	//
	// Optional (documentation only): the status actually returned comes from the
	// error Handle returns (see HTTPError), not from this declaration.
	Errors []ErrorSpec

	// --- Who may call this endpoint ---

	// Security declares whether the endpoint needs a verified caller and what
	// that caller must hold.
	//
	// Optional (runtime), but leaving it out means "a verified caller is
	// required": an endpoint nobody declared is closed, not open. A public
	// endpoint therefore has to say so explicitly.
	Security *SecuritySpec

	// --- Behaviour that cannot be derived from the types ---

	// Behavior declares side effects, idempotency and preconditions.
	//
	// Optional (documentation only).
	Behavior BehaviorSpec

	// Handle receives the validated request body and returns a typed Result or an
	// error. Use rest.NewResult for a body, restkit.NoContent for an empty one.
	//
	// Required.
	Handle func(r *rest.Request, body *Req) (*rest.Result[Res], error)
}

// HeaderSpec declares one row of the response header table.
type HeaderSpec struct {
	// Name is the header name as sent on the wire. Required.
	Name string
	// Meaning explains what the caller should do with it. Optional.
	Meaning string
}

// ErrorSpec declares one error specific to an endpoint.
type ErrorSpec struct {
	// Status is the HTTP status of this error. Required.
	Status int
	// Code is the machine-readable code clients branch on. Required.
	Code string
	// Condition states when the error occurs. Optional, but generated documents
	// show TODO without it.
	Condition string
	// CallerAction states what the caller should do, including whether retrying
	// is safe. Optional, but generated documents show TODO without it.
	CallerAction string
	// BodyType is the type of this error's body, used to render its own example
	// and field table. Declare it when the error deviates from the common error
	// shape or carries field-level details.
	//
	// Optional: when nil the error is documented as a single table row that
	// follows the common error shape.
	BodyType reflect.Type
}

// BehaviorSpec declares facts about an endpoint that no type can express.
type BehaviorSpec struct {
	// SideEffects lists what the call changes besides its own response.
	// Declare "none" explicitly rather than leaving it empty, so that readers can
	// tell "nothing happens" from "nobody wrote it down".
	SideEffects []string
	// Idempotent states whether repeating the call is safe.
	// nil means undeclared, which generated documents surface as TODO.
	Idempotent *bool
	// Preconditions lists what must already be true for the call to succeed.
	Preconditions []string
}

// SecuritySpec declares who may call an endpoint.
//
// The zero value (and a nil *SecuritySpec) means a verified caller is required
// with no further restriction. That is deliberate: an endpoint whose author
// forgot to think about access ends up closed rather than open.
type SecuritySpec struct {
	// Public serves the endpoint without any caller. This is the only way to
	// open an endpoint, and it has to be written down.
	Public bool
	// Schemes limits which credentials are accepted (authkit.SchemeJWT,
	// authkit.SchemeAPIKey). Empty accepts any scheme the application verifies.
	Schemes []string
	// Scopes are the permissions the caller must hold — all of them, not any of
	// them, matching how OpenAPI reads a security requirement.
	Scopes []string
}

// ResponseSpec declares a success response other than the primary one
// (Success + Res): the 200-or-201 upsert, a 202 with a different body, and so on.
// None of this can be derived from the types, because the status is chosen inside
// Handle and the condition exists only in the developer's head.
type ResponseSpec struct {
	// Status is the HTTP status this response uses. Required.
	Status int
	// When states the condition that produces this status
	// ("the user already existed"). Optional, but generated documents fall back
	// to a generic description without it.
	When string
	// BodyType is the body type for this status. Optional: nil means no body,
	// which is what 204 responses want.
	BodyType reflect.Type
	// Headers documents the headers sent with this status. Optional, and like
	// Endpoint.ResponseHeaders it documents rather than sends them.
	Headers []HeaderSpec
}

// EndpointDoc is the normalized, non-generic view of an endpoint's types and
// declarations that apidoc reads. Endpoint itself is generic and awkward to
// reflect over, so the adapter converts it into this shape.
type EndpointDoc struct {
	Summary           string
	Description       string
	FieldDocs         map[string]string
	RequestFieldDocs  map[string]string
	ResponseFieldDocs map[string]string
	IDPrefix          string
	Task              string
	AlsoRead          []string
	Related           []string

	ReqType reflect.Type
	ResType reflect.Type

	BodyRules  []*rule.RuleSet
	PathRules  []*rule.RuleSet
	QueryRules []*rule.RuleSet

	Success         int
	ResponseHeaders []HeaderSpec
	Produces        string
	Responses       []ResponseSpec
	Errors          []ErrorSpec
	Security        *SecuritySpec
	Behavior        BehaviorSpec
}

// Documented is satisfied by adapters that expose their documentation metadata.
// apidoc type-asserts each rest.Controller to this interface to collect it;
// controllers that do not implement it are documented as untyped.
type Documented interface {
	EndpointDoc() EndpointDoc
}

// NoBody is the Req type for endpoints that take no request body (GET, DELETE
// and friends), as in Endpoint[NoBody, dto.GetUser]. Read path and query values
// inside Handle with r.PathValue / r.Query, and declare their validation in
// PathRules / QueryRules.
//
// Endpoints with no response body (204 No Content and the like) use NoBody for
// Res as well and return NoContent() from Handle, as in Endpoint[NoBody, NoBody].
type NoBody struct{}

// NoContent returns a response with no body (204 No Content, an empty DELETE
// response, and so on). It is a thin wrapper over rest.NoContent[NoBody] that
// saves writing the type argument.
//
//	// inside Handle of Endpoint[Req, restkit.NoBody]{ Success: http.StatusNoContent, ... }:
//	return restkit.NoContent(), nil
func NoContent(opts ...rest.ResultOption) *rest.Result[NoBody] {
	return rest.NoContent[NoBody](opts...)
}
