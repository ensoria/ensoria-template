package sessionkit

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// idBytes is how much randomness a session id carries.
//
// The id is the whole credential: whoever holds it is the session, so guessing
// one is signing in as somebody else. 256 bits puts that out of reach for good,
// rather than for now — and the id is never displayed, so its length costs
// nothing anyone will notice.
const idBytes = 32

// newID returns a fresh session id.
//
// The encoding is unpadded base64url, whose alphabet is already legal in a
// cookie value, so nothing downstream has to escape or quote it.
//
// An error here means the operating system's randomness is unavailable, which
// is not a condition to carry on through: the alternative to a random id is a
// guessable one.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("sessionkit: generating a session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
