package keystore_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/keystore"
)

var _ = Describe("Fingerprint", func() {
	It("is the same for the same key", func() {
		Expect(keystore.Fingerprint("a-key")).To(Equal(keystore.Fingerprint("a-key")))
	})

	It("differs for different keys", func() {
		Expect(keystore.Fingerprint("a-key")).NotTo(Equal(keystore.Fingerprint("another-key")))
	})

	// Whatever writes a key into the store — a migration, an administration
	// tool — computes this value, so its shape is part of the contract.
	It("is lowercase hex of a SHA-256", func() {
		Expect(keystore.Fingerprint("a-key")).To(MatchRegexp(`^[0-9a-f]{64}$`))
	})

	// The point of storing it: a dump of the store yields nothing usable.
	It("does not contain the key", func() {
		Expect(keystore.Fingerprint("a-key")).NotTo(ContainSubstring("a-key"))
	})
})

var _ = Describe("NewKey", func() {
	It("issues a key with the fingerprint to store for it", func() {
		key, fingerprint, err := keystore.NewKey()

		Expect(err).NotTo(HaveOccurred())
		Expect(fingerprint).To(Equal(keystore.Fingerprint(key)))
	})

	It("never issues the same key twice", func() {
		first, _, err := keystore.NewKey()
		Expect(err).NotTo(HaveOccurred())
		second, _, err := keystore.NewKey()
		Expect(err).NotTo(HaveOccurred())

		Expect(first).NotTo(Equal(second))
	})

	// 32 random bytes in unpadded base64url. The key is the whole credential,
	// so guessing one is authenticating as its owner.
	It("issues a key long enough to be worth issuing", func() {
		key, _, err := keystore.NewKey()

		Expect(err).NotTo(HaveOccurred())
		Expect(key).To(HaveLen(43))
	})
})
