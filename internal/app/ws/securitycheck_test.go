package ws_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	wsApp "github.com/ensoria/ensoria-template/internal/app/ws"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/websocket/pkg/wsconfig"
)

// channel builds a typed channel module on path with the given declaration
// (test helper).
func channel(path string, security *wskit.SecuritySpec) *wskit.Module {
	return wskit.NewModule(&wskit.Channel{Path: path, Security: security})
}

var _ = Describe("checkChannelSecurity", func() {
	sessionsOnly := verifierFor{authkit.SchemeSession}
	public := &wskit.SecuritySpec{Public: true}

	It("accepts having no channels at all", func() {
		Expect(wsApp.CheckChannelSecurity(nil, nil)).To(Succeed())
	})

	It("accepts channels that need a caller when something can verify one", func() {
		modules := []*wskit.Module{
			channel("/ws/any", nil),
			channel("/ws/sessions", &wskit.SecuritySpec{Schemes: []string{authkit.SchemeSession}}),
		}

		Expect(wsApp.CheckChannelSecurity(modules, sessionsOnly)).To(Succeed())
	})

	Describe("a public channel that also states a condition", func() {
		// A public channel admits everyone before its conditions are read, so
		// they would never apply: it fails open.
		DescribeTable("is refused",
			func(security *wskit.SecuritySpec) {
				err := wsApp.CheckChannelSecurity([]*wskit.Module{channel("/ws/one", security)}, sessionsOnly)

				Expect(err).To(MatchError(ContainSubstring(`"/ws/one"`)))
				Expect(err).To(MatchError(ContainSubstring("drop Public")))
			},
			Entry("a scope", &wskit.SecuritySpec{Public: true, Scopes: []string{"things:read"}}),
			Entry("a scheme", &wskit.SecuritySpec{Public: true, Schemes: []string{authkit.SchemeSession}}),
		)

		It("is named along with every other one, not only the first", func() {
			modules := []*wskit.Module{
				channel("/ws/a", &wskit.SecuritySpec{Public: true, Scopes: []string{"things:read"}}),
				channel("/ws/fine", public),
				channel("/ws/b", &wskit.SecuritySpec{Public: true, Schemes: []string{authkit.SchemeJWT}}),
			}

			err := wsApp.CheckChannelSecurity(modules, sessionsOnly)

			Expect(err).To(MatchError(ContainSubstring(`"/ws/a", "/ws/b"`)))
			Expect(err).NotTo(MatchError(ContainSubstring("/ws/fine")))
		})

		// The contradiction is about the declarations alone, so it holds
		// whatever the configuration can verify, and it is the mistake worth
		// reporting first.
		It("is reported before the question of what can be verified", func() {
			modules := []*wskit.Module{
				channel("/ws/closed", nil),
				channel("/ws/public", &wskit.SecuritySpec{Public: true, Scopes: []string{"things:read"}}),
			}

			err := wsApp.CheckChannelSecurity(modules, verifierFor{})

			Expect(err).To(MatchError(ContainSubstring("drop Public")))
			Expect(err).NotTo(MatchError(ContainSubstring("nothing can verify")))
		})
	})

	Describe("channels that need a caller in an application that can verify none", func() {
		It("are refused, and only they are named", func() {
			modules := []*wskit.Module{
				channel("/ws/closed", nil),
				channel("/ws/public", public),
			}

			err := wsApp.CheckChannelSecurity(modules, verifierFor{})

			Expect(err).To(MatchError(ContainSubstring(`"/ws/closed"`)))
			Expect(err).To(MatchError(ContainSubstring("nothing can verify")))
			Expect(err).NotTo(MatchError(ContainSubstring("/ws/public")))
		})

		It("are refused when there is no verifier at all", func() {
			err := wsApp.CheckChannelSecurity([]*wskit.Module{channel("/ws/closed", nil)}, nil)

			Expect(err).To(MatchError(ContainSubstring("nothing can verify")))
		})

		It("are found among raw channels too", func() {
			modules := []*wskit.Module{wskit.Raw(wsconfig.NewDefaultModule("/ws/raw"), nil)}

			err := wsApp.CheckChannelSecurity(modules, verifierFor{})

			Expect(err).To(MatchError(ContainSubstring(`"/ws/raw"`)))
		})

		It("leave an application whose channels are all public alone", func() {
			modules := []*wskit.Module{channel("/ws/public", public)}

			Expect(wsApp.CheckChannelSecurity(modules, verifierFor{})).To(Succeed())
		})
	})

	// Requiring a kind of credential nothing verifies closes the channel to
	// everyone.
	Describe("a channel that accepts only kinds of credential nothing verifies", func() {
		It("is refused, naming each such kind next to its channel and what is available", func() {
			modules := []*wskit.Module{
				channel("/ws/a", &wskit.SecuritySpec{Schemes: []string{authkit.SchemeAPIKey, authkit.SchemeSession}}),
				channel("/ws/fine", &wskit.SecuritySpec{Schemes: []string{authkit.SchemeSession}}),
				channel("/ws/b", &wskit.SecuritySpec{Schemes: []string{authkit.SchemeJWT}}),
			}

			err := wsApp.CheckChannelSecurity(modules, sessionsOnly)

			Expect(err).To(MatchError(ContainSubstring(
				fmt.Sprintf(`"/ws/a" (%q), "/ws/b" (%q)`, authkit.SchemeAPIKey, authkit.SchemeJWT))))
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("(available: %q)", authkit.SchemeSession))))
			Expect(err).NotTo(MatchError(ContainSubstring("/ws/fine")))
		})
	})
})
