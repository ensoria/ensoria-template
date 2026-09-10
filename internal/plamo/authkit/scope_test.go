package authkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// implications builds a table the specs can rely on having been accepted.
func implications(table map[string][]string) *authkit.ScopeImplications {
	GinkgoHelper()
	resolved, err := authkit.NewScopeImplications(table)
	Expect(err).NotTo(HaveOccurred())
	return resolved
}

var _ = Describe("ScopeImplications", func() {
	Describe("resolving the table", func() {
		It("keeps what a scope directly implies", func() {
			resolved := implications(map[string][]string{
				"admin": {"orders:read", "orders:write"},
			})

			Expect(resolved.Closure()).To(Equal(map[string][]string{
				"admin": {"orders:read", "orders:write"},
			}))
		})

		// The chain is the reason the closure exists: without it, every scope
		// above manager would have to repeat manager's right-hand side, and a
		// copy that was not updated would go unnoticed.
		It("follows a chain to its end", func() {
			resolved := implications(map[string][]string{
				"admin":   {"manager"},
				"manager": {"orders:read"},
			})

			Expect(resolved.Closure()).To(Equal(map[string][]string{
				"admin":   {"manager", "orders:read"},
				"manager": {"orders:read"},
			}))
		})

		It("folds a scope listed twice on the same right-hand side", func() {
			resolved := implications(map[string][]string{
				"admin": {"orders:read", "orders:read"},
			})

			Expect(resolved.Closure()).To(Equal(map[string][]string{
				"admin": {"orders:read"},
			}))
		})

		// Writing a → a says nothing, so it is dropped rather than reported as
		// the cycle it technically is.
		It("ignores a scope that implies itself", func() {
			resolved := implications(map[string][]string{
				"admin": {"admin", "orders:read"},
			})

			Expect(resolved.Closure()).To(Equal(map[string][]string{
				"admin": {"orders:read"},
			}))
		})

		// A scope with an empty right-hand side is a row that says nothing, and
		// the generated documentation should not print it.
		It("leaves out a scope that implies nothing", func() {
			resolved := implications(map[string][]string{
				"admin":     {"orders:read"},
				"anonymous": {},
			})

			Expect(resolved.Closure()).To(HaveKey("admin"))
			Expect(resolved.Closure()).NotTo(HaveKey("anonymous"))
		})

		It("accepts an empty table, which expands nothing", func() {
			resolved := implications(nil)

			Expect(resolved.Closure()).To(BeEmpty())
			Expect(resolved.Expand([]string{"admin"})).To(Equal([]string{"admin"}))
		})

		It("hands back a copy, so a reader cannot edit the policy", func() {
			resolved := implications(map[string][]string{"admin": {"orders:read"}})

			resolved.Closure()["admin"][0] = "everything"

			Expect(resolved.Closure()).To(Equal(map[string][]string{
				"admin": {"orders:read"},
			}))
		})
	})

	Describe("a cycle", func() {
		// Two names for one permission is a mistake to fix, and the message has
		// to say which entry to delete — "there is a cycle" does not.
		It("is refused, naming the route that closes it", func() {
			_, err := authkit.NewScopeImplications(map[string][]string{
				"admin":   {"manager"},
				"manager": {"admin"},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("admin -> manager -> admin"))
		})

		It("is refused when the route is longer", func() {
			_, err := authkit.NewScopeImplications(map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": {"a"},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("a -> b -> c -> a"))
		})
	})

	Describe("a table that cannot mean anything", func() {
		It("is refused when a scope has no name", func() {
			_, err := authkit.NewScopeImplications(map[string][]string{"": {"orders:read"}})

			Expect(err).To(HaveOccurred())
		})

		It("is refused when a scope implies an empty name", func() {
			_, err := authkit.NewScopeImplications(map[string][]string{"admin": {""}})

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Expand", func() {
		var resolved *authkit.ScopeImplications

		BeforeEach(func() {
			resolved = implications(map[string][]string{
				"admin":   {"manager"},
				"manager": {"orders:read"},
			})
		})

		It("adds what the held scopes imply", func() {
			Expect(resolved.Expand([]string{"admin"})).
				To(Equal([]string{"admin", "manager", "orders:read"}))
		})

		It("leaves a scope the table says nothing about alone", func() {
			Expect(resolved.Expand([]string{"users:read"})).To(Equal([]string{"users:read"}))
		})

		It("does not repeat a scope the caller already held", func() {
			Expect(resolved.Expand([]string{"manager", "orders:read"})).
				To(Equal([]string{"manager", "orders:read"}))
		})

		// The result reaches the generated documentation, which has to come out
		// the same on every run.
		It("answers the same order every time", func() {
			first := resolved.Expand([]string{"admin", "users:read"})
			for i := 0; i < 20; i++ {
				Expect(resolved.Expand([]string{"admin", "users:read"})).To(Equal(first))
			}
		})

		// held belongs to a Principal, and a Principal is read by whatever comes
		// after this.
		It("does not write into the scopes it was given", func() {
			held := []string{"admin"}

			resolved.Expand(held)

			Expect(held).To(Equal([]string{"admin"}))
		})
	})
})
