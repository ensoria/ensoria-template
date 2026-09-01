package sessionkit

import (
	"net/http"
	"time"
)

// discardMaxAge is what net/http writes as `Max-Age=0`, the instruction to drop
// a cookie now. A zero MaxAge means something else entirely — omit the
// attribute — so it cannot be used here.
const discardMaxAge = -1

// Cookies writes the session cookie and the instruction to drop it.
//
// It is separate from Store because the two are needed in different places:
// whatever creates or ends a session writes a cookie, and the verifier writes a
// discard instruction for a session that turned out not to exist — without ever
// creating one.
//
// # What is not configurable, and why
//
// HttpOnly, Path and the absence of Domain are fixed. HttpOnly is the entire
// point: a cookie script can read is a token in localStorage with extra steps.
// Path narrower than / produces a cookie missing from the requests that needed
// it, and a Domain widens the cookie to every host under it — which is the
// sharing __Host- exists to prevent. None of the three has a development use,
// so none of them is a setting that could be got wrong.
type Cookies struct {
	cfg *Config
}

// NewCookies builds the cookie writer for a configuration.
func NewCookies(cfg *Config) *Cookies {
	return &Cookies{cfg: cfg}
}

// Name is the cookie the session id is carried under. The verifier reads it,
// and the generated documentation names it.
func (c *Cookies) Name() string {
	return c.cfg.CookieName
}

// Issue returns the cookie carrying a session.
//
// Max-Age is written only for a persistent session, and set to the session's
// absolute lifetime. Without it the browser keeps the cookie only until it
// closes, which is what makes the default profile a browser session — the
// server's deadline is unchanged either way, so a cookie that outlived its
// session would produce nothing but a request that is refused.
func (c *Cookies) Issue(session *Session) *http.Cookie {
	cookie := c.base()
	cookie.Value = session.ID
	if session.Persistent {
		cookie.MaxAge = int(c.cfg.AbsoluteTTLFor(true) / time.Second)
	}
	return cookie
}

// Discard returns the instruction to drop the cookie: the same cookie, empty,
// with Max-Age=0.
//
// The attributes have to match the ones it was written with, or the browser
// treats it as a different cookie and keeps sending the original — which is why
// this is built from the same base rather than assembled at each call site.
func (c *Cookies) Discard() *http.Cookie {
	cookie := c.base()
	cookie.Value = ""
	cookie.MaxAge = discardMaxAge
	return cookie
}

// base is the cookie both of the above start from: everything that identifies
// the cookie, and nothing that says what it holds.
func (c *Cookies) base() *http.Cookie {
	return &http.Cookie{
		Name:     c.cfg.CookieName,
		Path:     cookiePath,
		HttpOnly: true,
		Secure:   c.cfg.CookieSecure,
		SameSite: c.cfg.CookieSameSite,
	}
}
