package auth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

var _ = Describe("the application's scope policy", func() {
	// A cycle in the table stops the process, so the table shipped with the
	// template has to be one that starts.
	It("resolves", func() {
		expander, err := NewScopeExpander()

		Expect(err).NotTo(HaveOccurred())
		Expect(expander).NotTo(BeNil())
	})

	It("lets admin stand for every permission the application's endpoints and messages declare", func() {
		expander, err := NewScopeExpander()
		Expect(err).NotTo(HaveOccurred())

		caller := (&authkit.Principal{Scopes: []string{"admin"}}).WithScopeExpander(expander)

		Expect(caller.SatisfiesScopes(applicationScopes())).To(BeTrue())
	})

	// ⚠ Deliberate: operating the worker and the scheduler is a different role
	// from administering the application's data, so admin is not given it.
	It("does not let admin stand for the worker's or the scheduler's permissions", func() {
		expander, err := NewScopeExpander()
		Expect(err).NotTo(HaveOccurred())

		caller := (&authkit.Principal{Scopes: []string{"admin"}}).WithScopeExpander(expander)

		for _, scope := range operatorScopes() {
			Expect(caller.SatisfiesScopes([]string{scope})).To(BeFalse(), scope)
		}
	})

	// ⚠ Deliberate: the README teaches the difference between "authenticated"
	// and "permitted" with an API key that holds orders:write and is refused by
	// GET /order. An implication from orders:write to orders:read would be a
	// reasonable policy in a real project and would make that example stop
	// demonstrating anything, so it is not in the table.
	It("does not let orders:write stand for orders:read", func() {
		expander, err := NewScopeExpander()
		Expect(err).NotTo(HaveOccurred())

		caller := (&authkit.Principal{Scopes: []string{"orders:write"}}).WithScopeExpander(expander)

		Expect(caller.SatisfiesScopes([]string{"orders:read"})).To(BeFalse())
	})

	Describe("ScopeImplicationTable", func() {
		// The generated documentation reads it, and a renderer must not be able
		// to edit the policy by editing what it was shown.
		It("hands back a copy", func() {
			table := ScopeImplicationTable()
			table["admin"] = []string{"everything"}

			Expect(ScopeImplicationTable()["admin"]).NotTo(ContainElement("everything"))
		})

		It("says the same thing the resolved policy enforces", func() {
			expander, err := authkit.NewScopeImplications(ScopeImplicationTable())
			Expect(err).NotTo(HaveOccurred())

			caller := (&authkit.Principal{Scopes: []string{"admin"}}).WithScopeExpander(expander)

			Expect(caller.SatisfiesScopes(applicationScopes())).To(BeTrue())
		})
	})
})
