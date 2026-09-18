package authkit

import (
	"encoding/json"
	"math"
	"reflect"
)

// maxExactInteger is the largest integer a float64 carries without losing a
// digit (2^53). Beyond it, several integers share one float64, so the value read
// back is not necessarily the one the issuer wrote.
const maxExactInteger = 1 << 53

// Claims are the claims a credential carried, beyond the subject and scopes the
// Principal names itself.
//
// # Reading them
//
// The accessors below answer with the (value, ok) pair that rest.Request uses
// for headers and query parameters: ok is false when the claim is absent, and
// also when it holds something other than what was asked for. A caller who needs
// to tell those apart, or who needs a shape the accessors do not cover (a nested
// object, say), can read the map directly — it is a plain map[string]any.
//
//	org, ok := principal.Claims.String("org")
//	level, ok := principal.Claims.Int("level")
//	roles, ok := principal.Claims.StringSlice("roles")
//
// # Why they exist
//
// A claim usually reaches this application through JSON, and JSON has its own
// type system: every number arrives as a float64 and every array as a []any,
// however the issuer wrote it. That holds for a verified token, for a session
// restored from the store, and for a caller carried between services. So
//
//	level, ok := claims["level"].(int)
//
// never succeeds, whatever the token says — and it fails silently, by refusing a
// caller who should have been allowed. Doing that conversion correctly in one
// place, rather than at every rule that reads a claim, is what these are for.
//
// A claim built in Go and never serialized — one a KeyStore puts on the caller
// it returns — keeps its Go type instead. The accessors take both, so a rule
// does not have to know which kind of caller it is looking at.
//
// # What they will not do
//
// They convert between representations of the same value, never between values.
// float64(3), int(3) and json.Number("3") are the same number and all read as
// one; the string "3" is a different claim and reads as nothing. Likewise "true"
// is not a boolean and "admin" is not a one-element list.
//
// The reason is that the alternative has no correct answer. Whether "TRUE" or
// "1" or "yes" counts as true, or whether "admin editor" is one role or two, is
// a guess — and a guess made here becomes an authorization decision the issuer
// never asserted. Refusing is visible instead: the rule denies, which surfaces as
// a 403 while developing, and the fix (a mapper in the identity provider, or a
// deliberate conversion in the declaration) is made where it belongs.
//
// ⚠ An integer beyond 2^53 cannot survive a JWT this application verifies: the
// token carries the digits, and decoding lands them in a float64. Int reports
// such a value as absent rather than handing back a number that is off by one.
// An identity provider with ids that large should issue them as strings.
type Claims map[string]any

// String returns a string claim.
func (c Claims) String(name string) (string, bool) {
	value, present := c[name]
	if !present {
		return "", false
	}
	return stringOf(value)
}

// Bool returns a boolean claim.
func (c Claims) Bool(name string) (bool, bool) {
	value, present := c[name]
	if !present {
		return false, false
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Bool {
		return false, false
	}
	return v.Bool(), true
}

// Number returns a numeric claim as a float64, which is what JSON numbers are.
//
// Use Int for a claim that is meant to be a whole number: this one accepts 2.5
// as readily as 2, and truncating it afterwards is the mistake these accessors
// exist to prevent.
func (c Claims) Number(name string) (float64, bool) {
	value, present := c[name]
	if !present {
		return 0, false
	}
	return floatOf(value)
}

// Int returns a whole-number claim.
//
// A value with a fractional part is not a whole number and is refused rather
// than truncated. So is one too large for a float64 to hold exactly, because the
// digits it was written with are already lost by the time it arrives (see
// Claims).
func (c Claims) Int(name string) (int64, bool) {
	value, present := c[name]
	if !present {
		return 0, false
	}
	return intOf(value)
}

// StringSlice returns a claim holding a list of strings, such as the roles or
// groups an identity provider attaches.
//
// The result is a fresh slice, so a rule cannot change what the next request
// reads. A list with anything but strings in it is refused whole: returning the
// strings that happened to be in it would judge a caller on part of a claim.
func (c Claims) StringSlice(name string) ([]string, bool) {
	value, present := c[name]
	if !present {
		return nil, false
	}

	list := reflect.ValueOf(value)
	if list.Kind() != reflect.Slice {
		return nil, false
	}

	values := make([]string, 0, list.Len())
	for index := range list.Len() {
		// A []any holds interfaces; Interface() unwraps each element to the
		// value inside, which is what the elements of a decoded JSON array are.
		element, ok := stringOf(list.Index(index).Interface())
		if !ok {
			return nil, false
		}
		values = append(values, element)
	}
	return values, true
}

// stringOf reads a value as a string.
//
// The kinds are read through reflect so that a named type a KeyStore might use
// (type Tenant string) is not quietly refused — a refusal here ends as a denied
// request, which is the hardest kind of mistake to find. json.Number is the one
// string-kinded value excluded: it is how a number arrives, not a string claim.
func stringOf(value any) (string, bool) {
	if _, number := value.(json.Number); number {
		return "", false
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.String {
		return "", false
	}
	return v.String(), true
}

// floatOf reads a value as a float64.
func floatOf(value any) (float64, bool) {
	if number, ok := value.(json.Number); ok {
		parsed, err := number.Float64()
		return parsed, err == nil
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	default:
		return 0, false
	}
}

// intOf reads a value as a whole number.
func intOf(value any) (int64, bool) {
	if number, ok := value.(json.Number); ok {
		// Int64 keeps every digit of a number written as an integer, which is
		// what a claim parsed with json.Number is for. Anything else (3.0, or a
		// number too large for an int64) goes through the float rules.
		if parsed, err := number.Int64(); err == nil {
			return parsed, true
		}
		parsed, err := number.Float64()
		if err != nil {
			return 0, false
		}
		return intFromFloat(parsed)
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned := v.Uint()
		if unsigned > math.MaxInt64 {
			return 0, false
		}
		return int64(unsigned), true
	case reflect.Float32, reflect.Float64:
		return intFromFloat(v.Float())
	default:
		return 0, false
	}
}

// intFromFloat converts a float64 that is exactly a whole number.
func intFromFloat(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value != math.Trunc(value) {
		return 0, false
	}
	if value > maxExactInteger || value < -maxExactInteger {
		return 0, false
	}
	return int64(value), true
}
