package auth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the claims the application carries in an Initiator", func() {
	// A list naming a claim that must not be carried stops the process, so the
	// list shipped with the template has to be one that starts.
	It("builds", func() {
		set, err := NewInitiatorClaims()

		Expect(err).NotTo(HaveOccurred())
		Expect(set).NotTo(BeNil())
	})

	// Nothing leaves the service unless someone decides it should: a claim
	// added here is personal data travelling to every consumer.
	It("carries nothing by default", func() {
		set, err := NewInitiatorClaims()
		Expect(err).NotTo(HaveOccurred())

		Expect(set.Names()).To(BeEmpty())
		Expect(InitiatorClaimNames()).To(BeEmpty())
	})

	It("hands the documentation a copy of the list", func() {
		previous := initiatorClaims
		DeferCleanup(func() { initiatorClaims = previous })
		initiatorClaims = []string{"tenant"}

		names := InitiatorClaimNames()
		names[0] = "changed"

		Expect(InitiatorClaimNames()).To(Equal([]string{"tenant"}))
	})
})
