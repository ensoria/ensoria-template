package keystore

import (
	"context"
	"errors"
	"fmt"
	"slices"

	enscache "github.com/ensoria/cache/pkg/cache"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// recordKeyPrefix namespaces the key records within whatever prefix the cache
// was built with.
const recordKeyPrefix = "apikey:"

// errUnusableRecord marks a record that was read but says nothing usable.
//
// It is deliberately not authkit.ErrKeyNotFound: the key exists, and answering
// 401 would send its owner off to check a key that is perfectly correct. It is
// a fault in the stored data, which is this side's problem — so it becomes a
// 5xx and lands in the logs, where somebody can fix the record.
var errUnusableRecord = errors.New("keystore: the stored record is unusable")

// redisStore answers key lookups from a cache.Cache.
//
// Any cache.Cache will do, which is what makes cachememory usable in a test
// without a second implementation to keep in step. A deployment gives it
// cacheredis.
//
// ⚠ Never hand it cachetiered or cacheotter. Withdrawing a key has to take
// effect everywhere at once, and a process-local copy of a record keeps
// answering with a key that was deleted — on that node, until the copy expires.
type redisStore struct {
	cache enscache.Cache
}

// NewRedis reads keys from the given cache.
func NewRedis(cache enscache.Cache) (authkit.KeyStore, error) {
	if cache == nil {
		return nil, errors.New("keystore: no cache to read keys from")
	}
	return &redisStore{cache: cache}, nil
}

// Lookup returns the caller a key belongs to.
//
// The key never reaches storage or an error message: only its fingerprint does.
// An unknown key is reported as authkit.ErrKeyNotFound, and everything else as
// an error meaning the store could not be asked — the difference between a 401
// and a 5xx, and between one caller being told no and every caller being told
// no during an outage.
func (s *redisStore) Lookup(ctx context.Context, key string) (*authkit.Principal, error) {
	if key == "" {
		return nil, authkit.ErrKeyNotFound
	}

	fingerprint := Fingerprint(key)
	record, err := enscache.Get[*Record](ctx, s.cache, recordKeyPrefix+fingerprint)
	if err != nil {
		if errors.Is(err, enscache.ErrCacheMiss) {
			return nil, authkit.ErrKeyNotFound
		}
		return nil, fmt.Errorf("keystore: reading the key record %s: %w", short(fingerprint), err)
	}
	if err := record.validate(); err != nil {
		return nil, fmt.Errorf("%w (fingerprint %s)", err, short(fingerprint))
	}

	return &authkit.Principal{
		Subject: record.Subject,
		Scopes:  slices.Clone(record.Scopes),
		Scheme:  authkit.SchemeAPIKey,
	}, nil
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
