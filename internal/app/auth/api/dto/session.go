// Package dto holds the request and response bodies of the session exchange
// endpoints.
package dto

import "time"

// CreateSession is the body of POST /session.
//
// It carries one field on purpose. Everything else about the session — who it
// belongs to, what it may do, how long it lives — comes from the token the
// caller presented and from the application's configuration, so there is
// nothing here a caller could set to give themselves more than the token
// already granted.
type CreateSession struct {
	// Persistent asks for the longer lifetime profile: the "keep me signed in"
	// box on a sign-in form.
	//
	// Left out (the default) the session lasts the shorter profile and its
	// cookie is dropped when the browser closes, which is what a shared
	// computer needs.
	Persistent bool `json:"persistent"`
}

// Session is what a caller is told about the session they now hold.
//
// ⚠ The session id is deliberately absent, and putting it back would undo the
// point of the whole design. The id travels in an HttpOnly cookie precisely so
// that script running in the page cannot read it; returning the same value in a
// response body hands it to any script that can call fetch, which is every
// script injected into the page.
type Session struct {
	// Subject is who the session belongs to, taken from the token that was
	// exchanged. A frontend usually already knows it; it is echoed so that the
	// browser and the server cannot silently disagree about who is signed in.
	Subject string `json:"subject"`
	// Persistent is the lifetime profile the session was created with, which is
	// what the caller asked for.
	Persistent bool `json:"persistent"`
	// ExpiresAt is the session's absolute deadline: the moment signing in again
	// is required however active the session has been.
	//
	// ⚠ It is not the only deadline. A session also ends after the configured
	// idle limit without a request, and that one is not reported — it moves
	// forward every time the session is used, so a value sent now would be
	// wrong by the next request.
	ExpiresAt time.Time `json:"expires_at"`
}
