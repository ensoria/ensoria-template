package service

import (
	"context"
	"errors"
	"time"

	"github.com/ensoria/cache/pkg/cachememory"
	"github.com/ensoria/ensoria-template/internal/query/user_post/record"
	repositorymock "github.com/ensoria/ensoria-template/internal/query/user_post/repository/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The point of caching this read is that the repository behind it stops being
// consulted. These specs use a real in-memory cache rather than a mocked one,
// so what they check is the behaviour a request actually gets — not that a
// particular cache method was called.
var _ = Describe("UserPostService.GetByID", func() {
	var (
		ctx        context.Context
		repository *repositorymock.UserPostRepositoryMock
		service    UserPostService
	)

	stored := &record.UserPostRecord{
		ID:        42,
		CreatedAt: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
	}

	BeforeEach(func() {
		ctx = context.Background()
		repository = repositorymock.NewUserPostRepositoryMock()
		// A cache of its own per spec, so one spec's entries cannot answer
		// another's read.
		service = NewUserPostService(repository, cachememory.New("test"))
	})

	Describe("a first read", func() {
		It("asks the repository and returns what it gave", func() {
			repository.WillReturn("GetByID", stored, nil)

			got, err := service.GetByID(ctx, 42)

			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(stored))
			Expect(repository.WasCalledTimes("GetByID", 1)).To(BeTrue())
		})
	})

	Describe("a second read of the same record", func() {
		It("does not ask the repository again", func() {
			repository.WillReturn("GetByID", stored, nil)

			_, err := service.GetByID(ctx, 42)
			Expect(err).NotTo(HaveOccurred())

			_, err = service.GetByID(ctx, 42)
			Expect(err).NotTo(HaveOccurred())

			Expect(repository.WasCalledTimes("GetByID", 1)).To(BeTrue(),
				"the second read must be answered from the cache")
		})

		It("answers with the same record", func() {
			repository.WillReturn("GetByID", stored, nil)

			first, err := service.GetByID(ctx, 42)
			Expect(err).NotTo(HaveOccurred())

			second, err := service.GetByID(ctx, 42)
			Expect(err).NotTo(HaveOccurred())

			Expect(second).To(Equal(first))
		})
	})

	// Two ids are two values, so they must not share an entry.
	Describe("reads of different records", func() {
		It("asks the repository once for each", func() {
			repository.WillReturnFunc("GetByID", func(args ...any) []any {
				id := args[1].(int64)
				return []any{&record.UserPostRecord{ID: id}, nil}
			})

			first, err := service.GetByID(ctx, 1)
			Expect(err).NotTo(HaveOccurred())

			second, err := service.GetByID(ctx, 2)
			Expect(err).NotTo(HaveOccurred())

			Expect(first.ID).To(Equal(int64(1)))
			Expect(second.ID).To(Equal(int64(2)))
			Expect(repository.WasCalledTimes("GetByID", 2)).To(BeTrue())
		})
	})

	// A failed read must not become a cached answer, or one unlucky moment
	// would be served for as long as the entry lives.
	Describe("a repository that fails", func() {
		It("returns the error", func() {
			repository.WillReturn("GetByID", nil, errors.New("the database is unreachable"))

			_, err := service.GetByID(ctx, 42)

			Expect(err).To(MatchError(ContainSubstring("the database is unreachable")))
		})

		It("caches nothing, so the next read tries again", func() {
			repository.WillReturnOnce("GetByID", nil, errors.New("the database is unreachable"))
			repository.WillReturn("GetByID", stored, nil)

			_, err := service.GetByID(ctx, 42)
			Expect(err).To(HaveOccurred())

			got, err := service.GetByID(ctx, 42)

			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(stored))
			Expect(repository.WasCalledTimes("GetByID", 2)).To(BeTrue())
		})
	})

	It("passes the caller's context to the repository", func() {
		type ctxKey struct{}
		marked := context.WithValue(ctx, ctxKey{}, "carried")

		var seen context.Context
		repository.WillReturnFunc("GetByID", func(args ...any) []any {
			seen = args[0].(context.Context)
			return []any{stored, nil}
		})

		_, err := service.GetByID(marked, 42)

		Expect(err).NotTo(HaveOccurred())
		Expect(seen.Value(ctxKey{})).To(Equal("carried"))
	})
})

// The key is what decides which reads share an answer, so its shape is part of
// the module's behaviour rather than an implementation detail.
var _ = Describe("cacheKey", func() {
	It("names the module and the id", func() {
		Expect(cacheKey(42)).To(Equal("user_post:42"))
	})

	It("gives different ids different keys", func() {
		Expect(cacheKey(1)).NotTo(Equal(cacheKey(2)))
	})
})
