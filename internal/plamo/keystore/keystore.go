// Package keystore is the API key store the framework ships: keys kept outside
// the configuration, so that they identify their caller, carry permissions of
// their own, and can be withdrawn without a restart.
//
// It is what AUTH_KEYSTORE configures. A project whose keys live somewhere this
// package has never heard of implements authkit.KeyStore instead and hands it
// to the verifier, which is what AUTH_API_KEYS_EXTERNAL declares.
//
// # Keys are stored by fingerprint, never as themselves
//
// An API key is a bearer credential: whoever reads it can use it. So the store
// holds Fingerprint(key) rather than the key, and a lookup fingerprints what
// arrived and compares that. A copy of the store — a database dump, a Redis
// snapshot, a backup on somebody's laptop — then contains nothing anyone can
// authenticate with.
//
// The cost lands on whoever issues a key: the value goes into the store as a
// fingerprint, and the key itself is shown to its owner once and never again,
// because nothing here can recover it. That is the same bargain a password
// hash makes, and it is worth the same.
//
// A plain SHA-256 is the right hash for this and would be the wrong one for a
// password. What makes a password need bcrypt or argon2 is that people choose
// passwords a machine can guess; the work factor buys time against that
// guessing. An API key issued by NewKey is 256 bits of randomness, so there is
// no guessing to slow down — and a fast hash is what lets the fingerprint be
// computed on every request without a thought.
package keystore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// keyBytes is how much randomness an issued API key carries. The key is the
// whole credential, so this is the same 256 bits a session id gets.
const keyBytes = 32

// Record is what the store holds for one key: who the key belongs to and what
// it may do.
//
// ⚠ The JSON tags are the format an operator writes when adding a key by hand,
// and records outlive a deployment, so renaming one invalidates every key
// already stored.
type Record struct {
	// Subject identifies the caller. It appears in logs and in every
	// authorization decision, so a key without one is refused: "some API key"
	// is not an answer to who did this.
	Subject string `json:"sub"`
	// Scopes are the permissions the key carries. Empty means the key
	// authenticates and is permitted nothing that a scope is declared for,
	// which is a legitimate answer for a caller that only reaches public
	// endpoints.
	Scopes []string `json:"scopes,omitempty"`
}

// Fingerprint is how a key is written down: the SHA-256 of the key, in
// lowercase hex.
//
// It is exported because issuing a key happens outside this application —
// a migration, an administration tool, a one-off command — and whatever does it
// has to compute the same value the lookup will.
func Fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// NewKey returns a fresh API key to hand to a caller, and the fingerprint to
// store for it.
//
// The key is returned once. Nothing can recover it from the fingerprint
// afterwards, which is the property that makes storing the fingerprint worth
// doing — so whatever issues a key has to show it to its owner there and then.
func NewKey() (key, fingerprint string, err error) {
	buf := make([]byte, keyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("keystore: generating an API key: %w", err)
	}
	key = base64.RawURLEncoding.EncodeToString(buf)
	return key, Fingerprint(key), nil
}

// validate reports a record that cannot identify its caller.
func (r *Record) validate() error {
	if r == nil || r.Subject == "" {
		return fmt.Errorf("keystore: the stored key has no subject: %w", errUnusableRecord)
	}
	return nil
}
