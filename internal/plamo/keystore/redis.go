package keystore

import (
	"context"
	"errors"
	"fmt"
	"slices"

	enscache "github.com/ensoria/cache/pkg/cache"
	enclikeystore "github.com/ensoria/encli/pkg/keystore"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
)

// redisStore answers key lookups from a cache.Cache.
//
// Any cache.Cache will do, which is what makes cachememory usable in a test
// without a second implementation to keep in step. A deployment gives it
// cacheredis, built with enclikeystore.RedisNamespace as its key prefix so that
// the records this reads are the ones encli writes.
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

	fingerprint := enclikeystore.Fingerprint(key)
	record, err := enscache.Get[*enclikeystore.Record](ctx, s.cache, enclikeystore.RedisKeyPrefix+fingerprint)
	if err != nil {
		if errors.Is(err, enscache.ErrCacheMiss) {
			return nil, authkit.ErrKeyNotFound
		}
		return nil, fmt.Errorf("keystore: reading the key record %s: %w", short(fingerprint), err)
	}
	if record == nil {
		return nil, fmt.Errorf("%w: the stored record is empty (fingerprint %s)",
			errUnusableRecord, short(fingerprint))
	}
	if err := validateSubject(record.Subject, fingerprint); err != nil {
		return nil, err
	}

	return &authkit.Principal{
		Subject: record.Subject,
		Scopes:  slices.Clone(record.Scopes),
		Scheme:  authkit.SchemeAPIKey,
	}, nil
}
