package backend

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/goswap/stats-api/models"
	"github.com/treeder/gotils"
)

// idea: do now % to and if we're outside of interval, go fetch the data again.
// issue being for daily we probably would rather update every hour. we could set
// the cache to just blow out every hour too (or 10 minutes or whatever).
//
// key prefix format:
// -------------------------------------------
// 1 byte endpoint id  | 8 bytes from | 8 bytes to | 8 bytes interval | N bytes rest of key
//                       (use 0 if n/a)
//
// IMPORTANT!!!:
// * if we round from and to then the cache doesn't get blown up, it's not perfect
// but in practice it's very good and only adds an extra cache miss when rounding goes up rather
// than down. TODO we could let caller do this but meh?
// * all cache entries MUST use fixed key size, this doesn't effect performance much and
// callers can send in mostly zero values easily enough

type epID uint8

const (
	pairEP epID = 1 + iota
	pairsEP
	tokenEP
	tokensEP
	totalsEP
	tokenBucketEP
	pairBucketEP
)

func key(endpoint epID, from, to time.Time, interval time.Duration, key string) string {
	// TODO(reed): align this, probably truncate interval
	var prefix [25]byte

	// Round works with 0, it just shaves off monotonic clock
	from = from.Round(interval)
	to = to.Round(interval)

	// we only have 8 bits, little endian so prefix[0] = epID, then overwrite prefix[1]
	binary.LittleEndian.PutUint16(prefix[:2], uint16(endpoint))

	binary.LittleEndian.PutUint64(prefix[1:9], uint64(from.Unix()))
	binary.LittleEndian.PutUint64(prefix[9:17], uint64(to.Unix()))
	binary.LittleEndian.PutUint64(prefix[17:], uint64(interval))

	return string(prefix[:]) + key
}

type cache struct {
	cache *ristretto.Cache
	ttl   time.Duration

	db StatsBackend
}

// compiler yelling
var cacheType StatsBackend = new(cache)

// NewCacheBackend returns a caching stats backend wrapping the given stats backend
func NewCacheBackend(ctx context.Context, db StatsBackend, ttl time.Duration) (StatsBackend, error) {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,             // number of keys to track frequency of (10M).
		MaxCost:     100 << (10 * 2), // maximum cost of cache (100MB), cloud run has 256MB by default
		BufferItems: 64,              // number of keys per Get buffer.
	})
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on NewCache: %v", err)
	}

	return &cache{
		cache: c,
		db:    db,
		ttl:   ttl,
	}, nil
}

// TODO(reed): meh, this doesn't help much and obviates / adds func stack alloc, keep toying with this
func (c *cache) check(key string, fill func() (interface{}, error)) (interface{}, error) {
	if v, ok := c.cache.Get(key); ok {
		return v, nil
	}

	v, err := fill()
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(key, v, 0, c.ttl)
	return v, nil
}

func (c *cache) GetPairs(ctx context.Context) ([]*models.Pair, error) {
	k := key(pairsEP, time.Time{}, time.Time{}, 0, "")
	if v, ok := c.cache.Get(k); ok {
		return v.([]*models.Pair), nil
	}

	pairs, err := c.db.GetPairs(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, pairs, 0, c.ttl)
	return pairs, nil
}

func (c *cache) GetPair(ctx context.Context, address string) (*models.Pair, error) {
	k := key(pairEP, time.Time{}, time.Time{}, 0, address)
	if v, ok := c.cache.Get(k); ok {
		return v.(*models.Pair), nil
	}

	pair, err := c.db.GetPair(ctx, address)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, pair, 0, c.ttl)
	return pair, nil
}

func (c *cache) GetTokens(ctx context.Context) ([]*models.Token, error) {
	k := key(tokensEP, time.Time{}, time.Time{}, 0, "")
	if v, ok := c.cache.Get(k); ok {
		return v.([]*models.Token), nil
	}

	tokens, err := c.db.GetTokens(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, tokens, 0, c.ttl)
	return tokens, nil
}

func (c *cache) GetToken(ctx context.Context, address string) (*models.Token, error) {
	k := key(tokenEP, time.Time{}, time.Time{}, 0, address)
	if v, ok := c.cache.Get(k); ok {
		return v.(*models.Token), nil
	}

	token, err := c.db.GetToken(ctx, address)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, token, 0, c.ttl)
	return token, nil
}

func (c *cache) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	k := key(totalsEP, from, to, interval, "")
	if v, ok := c.cache.Get(k); ok {
		return v.([]*models.TotalBucket), nil
	}

	totals, err := c.db.GetTotals(ctx, from, to, interval)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, totals, 0, c.ttl)
	return totals, nil
}

func (c *cache) GetPairBuckets(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	k := key(pairBucketEP, from, to, interval, pair)
	if v, ok := c.cache.Get(k); ok {
		return v.([]*models.PairBucket), nil
	}

	pairs, err := c.db.GetPairBuckets(ctx, pair, from, to, interval)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, pairs, 0, c.ttl)
	return pairs, nil
}

func (c *cache) GetTokenBuckets(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	k := key(tokenBucketEP, from, to, interval, token)
	if v, ok := c.cache.Get(k); ok {
		return v.([]*models.TokenBucket), nil
	}

	tokens, err := c.db.GetTokenBuckets(ctx, token, from, to, interval)
	if err != nil {
		return nil, err
	}

	c.cache.SetWithTTL(k, tokens, 0, c.ttl)
	return tokens, nil
}
