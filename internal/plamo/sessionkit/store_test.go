package sessionkit_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/ensoria-template/internal/plamo/sessionkit"
)

var _ = Describe("Store", func() {
	var (
		ctx   context.Context
		tick  *clock
		cache enscache.Cache
		store sessionkit.Store
	)

	// build wires a store over the given cache, on the hand-wound clock.
	build := func(c enscache.Cache) sessionkit.Store {
		GinkgoHelper()

		s, err := sessionkit.NewStore(c, testConfig(), sessionkit.WithClock(tick.now))
		Expect(err).NotTo(HaveOccurred())
		return s
	}

	BeforeEach(func() {
		ctx = context.Background()
		tick = newClock()
		cache = newMemoryCache()
		store = build(cache)
	})

	Describe("NewStore", func() {
		It("refuses a configuration that cannot produce a working cookie", func() {
			cfg := testConfig()
			cfg.CookieSecure = false // with the __Host- name still in place

			_, err := sessionkit.NewStore(cache, cfg)

			Expect(err).To(MatchError(ContainSubstring("__Host-")))
		})

		It("refuses to keep sessions nowhere", func() {
			_, err := sessionkit.NewStore(nil, testConfig())

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Create", func() {
		It("records the caller and both deadlines", func() {
			session, err := store.Create(ctx, snapshotOf("usr_1"), false)

			Expect(err).NotTo(HaveOccurred())
			Expect(session.Snapshot.Subject).To(Equal("usr_1"))
			Expect(session.Snapshot.Scopes).To(Equal([]string{"orders:read"}))
			Expect(session.CreatedAt).To(Equal(tick.now()))
			Expect(session.ExpiresAt).To(Equal(tick.now().Add(testAbsoluteTTL)))
			Expect(session.LastSeenAt).To(Equal(tick.now()))
		})

		It("gives the persistent profile the longer deadline", func() {
			session, err := store.Create(ctx, snapshotOf("usr_1"), true)

			Expect(err).NotTo(HaveOccurred())
			Expect(session.Persistent).To(BeTrue())
			Expect(session.ExpiresAt).To(Equal(tick.now().Add(testPersistentAbsoluteTTL)))
		})

		// The id is the entire credential: whoever holds it is the session.
		It("issues an id that is long and never repeated", func() {
			first, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())
			second, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			Expect(first.ID).NotTo(Equal(second.ID))
			// 32 random bytes in unpadded base64url.
			Expect(first.ID).To(HaveLen(43))
		})

		// A caller that kept the snapshot it passed in must not be able to
		// rewrite the permissions of a live session through it.
		It("stores a copy of the snapshot", func() {
			snapshot := snapshotOf("usr_1")
			session, err := store.Create(ctx, snapshot, false)
			Expect(err).NotTo(HaveOccurred())

			snapshot.Scopes[0] = "orders:write"
			snapshot.Subject = "usr_2"

			found, err := store.Lookup(ctx, session.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Snapshot.Subject).To(Equal("usr_1"))
			Expect(found.Snapshot.Scopes).To(Equal([]string{"orders:read"}))
		})

		It("refuses a session that belongs to nobody", func() {
			_, err := store.Create(ctx, &sessionkit.Snapshot{}, false)

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Lookup", func() {
		It("returns the session that was created", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			found, err := store.Lookup(ctx, created.ID)

			Expect(err).NotTo(HaveOccurred())
			Expect(found.ID).To(Equal(created.ID))
			Expect(found.Snapshot.Claims).To(HaveKeyWithValue("org", "acme"))
		})

		It("reports an id nobody was given as gone", func() {
			_, err := store.Lookup(ctx, "not-a-session-id")

			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		It("reports an empty id as gone without asking the store", func() {
			_, err := store.Lookup(ctx, "")

			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		It("stops honouring a session past its absolute deadline", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			// Used often enough that the idle deadline never binds.
			for range 5 {
				tick.advance(testIdleTTL / 2)
				_, err := store.Lookup(ctx, created.ID)
				Expect(err).NotTo(HaveOccurred())
			}
			tick.advance(testAbsoluteTTL)

			_, err = store.Lookup(ctx, created.ID)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		It("stops honouring a session nobody came back to", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			tick.advance(testIdleTTL)

			_, err = store.Lookup(ctx, created.ID)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		// The idle deadline is meant to reclaim sessions nobody uses, not to
		// sign out somebody who is using one.
		It("keeps a session in use alive past the idle limit", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), true)
			Expect(err).NotTo(HaveOccurred())

			for range 10 {
				tick.advance(testIdleTTL / 2)
				_, err := store.Lookup(ctx, created.ID)
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(tick.now()).To(BeTemporally(">", created.CreatedAt.Add(testIdleTTL)))
		})

		Describe("moving the idle deadline forward", func() {
			// Writing on every request would put a write in front of every
			// authenticated request, for a deadline measured in hours.
			It("leaves the record alone while it is still recent", func() {
				created, err := store.Create(ctx, snapshotOf("usr_1"), false)
				Expect(err).NotTo(HaveOccurred())

				tick.advance(testIdleRefresh - 1)
				found, err := store.Lookup(ctx, created.ID)

				Expect(err).NotTo(HaveOccurred())
				Expect(found.LastSeenAt).To(Equal(created.CreatedAt))
			})

			It("rewrites it once the record has fallen behind", func() {
				created, err := store.Create(ctx, snapshotOf("usr_1"), false)
				Expect(err).NotTo(HaveOccurred())

				tick.advance(testIdleRefresh)
				found, err := store.Lookup(ctx, created.ID)

				Expect(err).NotTo(HaveOccurred())
				Expect(found.LastSeenAt).To(Equal(tick.now()))
			})
		})
	})

	Describe("Revoke", func() {
		It("ends the session", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			Expect(store.Revoke(ctx, created.ID)).To(Succeed())

			_, err = store.Lookup(ctx, created.ID)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		// Signing out twice has given the caller what they asked for both times.
		It("succeeds for a session that is already gone", func() {
			Expect(store.Revoke(ctx, "not-a-session-id")).To(Succeed())
		})

		It("leaves the caller's other sessions alone", func() {
			phone, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())
			laptop, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			Expect(store.Revoke(ctx, phone.ID)).To(Succeed())

			_, err = store.Lookup(ctx, laptop.ID)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RevokeSubject", func() {
		It("ends every session the subject holds", func() {
			phone, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())
			laptop, err := store.Create(ctx, snapshotOf("usr_1"), true)
			Expect(err).NotTo(HaveOccurred())

			Expect(store.RevokeSubject(ctx, "usr_1")).To(Succeed())

			_, err = store.Lookup(ctx, phone.ID)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
			_, err = store.Lookup(ctx, laptop.ID)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		// The point of ending every session is to reach the ones this process
		// never saw, so a second process must honour a revocation it did not run.
		It("is honoured by a process that never saw the sessions", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			otherProcess := build(cache)
			Expect(otherProcess.RevokeSubject(ctx, "usr_1")).To(Succeed())

			_, err = store.Lookup(ctx, created.ID)
			Expect(err).To(MatchError(sessionkit.ErrSessionNotFound))
		})

		// Otherwise a forced sign-out would lock the person out until the
		// marker expired, which for the persistent profile is a matter of weeks.
		It("lets the subject sign in again straight away", func() {
			Expect(store.RevokeSubject(ctx, "usr_1")).To(Succeed())

			tick.advance(1)
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())

			found, err := store.Lookup(ctx, created.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.ID).To(Equal(created.ID))
		})

		It("leaves other subjects alone", func() {
			theirs, err := store.Create(ctx, snapshotOf("usr_2"), false)
			Expect(err).NotTo(HaveOccurred())

			Expect(store.RevokeSubject(ctx, "usr_1")).To(Succeed())

			_, err = store.Lookup(ctx, theirs.ID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("refuses to revoke the sessions of nobody", func() {
			Expect(store.RevokeSubject(ctx, "")).To(HaveOccurred())
		})
	})

	// The most damaging thing this package can get wrong. Reporting an outage as
	// "no such session" tells every browser to drop its cookie, and they do not
	// come back when the store returns.
	Describe("when the store cannot be reached", func() {
		It("does not report a lookup failure as a session that is gone", func() {
			down := build(newFailingCache(cache, sessionKeys))

			_, err := down.Lookup(ctx, "any-session-id")

			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(MatchError(sessionkit.ErrSessionNotFound))
		})

		// The subtle half: the record was read, so the session plainly exists.
		// Answering "not revoked" would keep serving sessions an administrator
		// has already ended, and answering "gone" would sign the caller out over
		// a read that never happened.
		It("refuses to guess when only the revocation marker is unreadable", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())
			down := build(newFailingCache(cache, revocationKeys))

			_, err = down.Lookup(ctx, created.ID)

			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(MatchError(sessionkit.ErrSessionNotFound))
		})

		It("reports a session it could not store", func() {
			down := build(newFailingCache(cache, anyKey))

			_, err := down.Create(ctx, snapshotOf("usr_1"), false)

			Expect(err).To(HaveOccurred())
		})

		// The id is the credential. A message repeating it in full puts a
		// working session into the logs.
		It("does not put the whole session id in the error", func() {
			created, err := store.Create(ctx, snapshotOf("usr_1"), false)
			Expect(err).NotTo(HaveOccurred())
			down := build(newFailingCache(cache, sessionKeys))

			_, err = down.Lookup(ctx, created.ID)

			Expect(err.Error()).NotTo(ContainSubstring(created.ID))
		})
	})
})
