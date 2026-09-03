// Package keystore is the API key store the framework ships: keys kept outside
// the configuration, so that they identify their caller, carry permissions of
// their own, and can be withdrawn without a restart.
//
// It is what AUTH_KEYSTORE configures. A project whose keys live somewhere this
// package has never heard of implements authkit.KeyStore instead and hands it
// to the verifier, which is what AUTH_API_KEYS_EXTERNAL declares.
//
// # Where the format lives, and why it is not here
//
// The storage format — how a key is written down, which table and columns,
// which Redis keys, how permissions are encoded — is
// [github.com/ensoria/encli/pkg/keystore]. It has to be, because two programs
// meet over it: encli creates the storage and issues keys into it, and this
// package reads it on every request that presents one. Writing the names out
// here as well would mean a rename on one side producing an application that
// starts cleanly and fails every lookup, on new installations only.
//
// So this package holds no names and no statements. What it holds is what the
// two programs do not share: turning a record into a caller, and deciding what
// a refusal means.
//
// # Keys are stored by fingerprint, never as themselves
//
// A lookup fingerprints the key that arrived and asks for that. A copy of the
// store — a database dump, a Redis snapshot, a backup on somebody's laptop —
// therefore contains nothing anyone can authenticate with, and nothing can
// recover a key that was lost. Issuing one is `encli auth keystore issue`; it
// cannot be done by writing the storage by hand.
package keystore

import (
	"errors"
	"fmt"
)

// errUnusableRecord marks a record that was read but says nothing usable.
//
// It is deliberately not authkit.ErrKeyNotFound: the key exists, and answering
// 401 would send its owner off to check a key that is perfectly correct. It is
// a fault in the stored data, which is this side's problem — so it becomes a
// 5xx and lands in the logs, where somebody can fix the record.
var errUnusableRecord = errors.New("keystore: the stored record is unusable")

// validateSubject reports a record that cannot identify its caller.
//
// A key with no subject would authenticate somebody the logs and every
// authorization decision could only call "some API key", which is not an answer
// to who did this.
func validateSubject(subject, fingerprint string) error {
	if subject == "" {
		return fmt.Errorf("%w: the stored key has no subject (fingerprint %s)",
			errUnusableRecord, short(fingerprint))
	}
	return nil
}

// fingerprintPrefix is how much of a fingerprint an error message carries:
// enough to find the record, and it is not a credential in any case.
const fingerprintPrefix = 12

// short trims a fingerprint for a message.
func short(fingerprint string) string {
	if len(fingerprint) <= fingerprintPrefix {
		return fingerprint
	}
	return fingerprint[:fingerprintPrefix] + "…"
}
