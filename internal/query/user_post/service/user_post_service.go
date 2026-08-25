package service

import (
	"context"
	"fmt"
	"time"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/ensoria-template/internal/query/user_post/record"
	"github.com/ensoria/ensoria-template/internal/query/user_post/repository"
)

// cacheTTL bounds how stale a cached record may be.
//
// It belongs to this module rather than to the cache: how long a post may be
// out of date is a question about posts. The tiered cache's own near TTL
// (CACHE_NEAR_TTL) is a different dial — it bounds how long one replica's
// in-process copy may differ from the shared one.
const cacheTTL = 5 * time.Minute

// cacheKeyNamespace separates this module's keys from every other module's.
//
// It is not the whole key a Redis instance sees: the store is built with
// cacheKeyPrefix ("app") and puts that in front, so a record lands under
// "app:user_post:42". This namespace only has to be unique among the modules of
// one application.
//
// TODO: derive this from the module name, together with the cacheKeyPrefix and
// cacheName TODO in internal/infra/cache/cache.go — writing the module's name
// out by hand here is exactly what that change would remove.
const cacheKeyNamespace = "user_post"

// cacheKey builds the key one record is cached under.
//
// A key has to name every input the value depends on. This value depends only
// on the id, so the id is all the key carries; a read that also varied by, say,
// a locale or a viewer would have to put those in the key too, or two different
// answers would overwrite each other.
func cacheKey(id int64) string {
	return fmt.Sprintf("%s:%d", cacheKeyNamespace, id)
}

//ensoria:mock
type UserPostService interface {
	GetByID(ctx context.Context, id int64) (*record.UserPostRecord, error)
}

func NewUserPostService(
	repository repository.UserPostRepository,
	cache enscache.Cache,
) *userPostService {
	return &userPostService{
		repository: repository,
		cache:      cache,
	}
}

type userPostService struct {
	repository repository.UserPostRepository
	cache      enscache.Cache
}

// GetByID reads one record, through the application cache.
//
// GetOrSetFunc is the shape a read-through cache wants: it returns the cached
// value on a hit and otherwise runs the function, stores what it returns, and
// hands it back. The repository is therefore called only on a miss, and only
// once per key even when several requests miss at the same time.
//
// The function is not called at all on a hit, which is why the repository can
// be expensive without every request paying for it. An error from it is
// returned as-is and nothing is stored, so a failed read never becomes a cached
// answer.
//
// The error result exists because of this cache: the record itself is a stub
// that cannot fail, but the L2 lives in Redis, and a caller has to hear about a
// Redis that is unreachable rather than silently reading through it.
func (s *userPostService) GetByID(ctx context.Context, id int64) (*record.UserPostRecord, error) {
	return enscache.GetOrSetFunc(ctx, s.cache, cacheKey(id), cacheTTL,
		func(ctx context.Context) (*record.UserPostRecord, error) {
			return s.repository.GetByID(ctx, id)
		})
}
