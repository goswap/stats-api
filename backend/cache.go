package backend

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/goswap/stats-api/models"
	"github.com/treeder/gotils"
)

// idea: round off 'from' and 'to' to the nearest interval, since most requests
// will be for a similar window at a given interval (eg last 24h, hourly), this
// will make most requests a cache hit until users can specify sliding windows
// (TODO).  certain endpoints like listing pairs we would like to be hot as
// well, and most key fields can be zero but all MUST use fixed key size to avoid
// conflicts with other endpoints. Rounding off means aside from ttl, we're adding
// 1 cache 'miss', we could always floor to fix this(TODO?), but it's pretty good.
//
// key prefix format:
// -------------------------------------------
// 1 byte endpoint id  | 8 bytes from | 8 bytes to | 8 bytes interval | N bytes rest of key
//                       (use 0 if n/a)

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

	// TODO(reed): we need this per key, really
	mu sync.RWMutex

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
	c.mu.RLock()
	v, ok := c.cache.Get(key)
	c.mu.RUnlock()
	if ok {
		return v, nil
	}

	// TODO probably shouldn't hold global mutex over db call.. just for MVP
	c.mu.Lock()
	defer c.mu.Unlock()

	// check again, first in the herd wins
	v, ok = c.cache.Get(key)
	if ok {
		return v, nil
	}

	// TODO we should probably set if there's an error too, or we'll blow the db up
	v, err := fill()
	if err != nil {
		return v, err
	}

	c.cache.SetWithTTL(key, v, 0, c.ttl)
	return v, nil
}

func (c *cache) GetPairs(ctx context.Context) ([]*models.Pair, error) {
	k := key(pairsEP, time.Time{}, time.Time{}, 0, "")
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetPairs(ctx)
	})
	pairs, _ := v.([]*models.Pair)
	return pairs, err
}

func (c *cache) GetPair(ctx context.Context, address string) (*models.Pair, error) {
	k := key(pairEP, time.Time{}, time.Time{}, 0, address)
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetPair(ctx, address)
	})
	pair, _ := v.(*models.Pair)
	return pair, err
}

func (c *cache) GetTokens(ctx context.Context) ([]*models.Token, error) {
	k := key(tokensEP, time.Time{}, time.Time{}, 0, "")
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetTokens(ctx)
	})
	tokens, _ := v.([]*models.Token)
	return tokens, err
}

func (c *cache) GetToken(ctx context.Context, address string) (*models.Token, error) {
	k := key(tokenEP, time.Time{}, time.Time{}, 0, address)
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetToken(ctx, address)
	})
	token, _ := v.(*models.Token)
	return token, err
}

func (c *cache) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	k := key(totalsEP, from, to, interval, "")
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetTotals(ctx, from, to, interval)
	})
	totals, _ := v.([]*models.TotalBucket)
	return totals, err
}

func (c *cache) GetPairBuckets(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	k := key(pairBucketEP, from, to, interval, pair)
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetPairBuckets(ctx, pair, from, to, interval)
	})
	pairs, _ := v.([]*models.PairBucket)
	return pairs, err
}

func (c *cache) GetTokenBuckets(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	k := key(tokenBucketEP, from, to, interval, token)
	v, err := c.check(k, func() (interface{}, error) {
		return c.db.GetTokenBuckets(ctx, token, from, to, interval)
	})
	tokens, _ := v.([]*models.TokenBucket)
	return tokens, err
}
