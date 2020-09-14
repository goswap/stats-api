package backend

import (
	"context"
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
// IMPORTANT!!!: if we round from and to then the cache doesn't get blown up
// TODO(reed): we could force the caller to round or just do it here. doesn't matter really but meh,
// it's more flexible if caller does it, here we have to guess and use interval?

type cache struct {
	cache *ristretto.Cache

	db StatsBackend
}

// NewCacheBackend returns a caching stats backend wrapping the given stats backend
func NewCacheBackend(ctx context.Context, db StatsBackend) (StatsBackend, error) {
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
	}, nil
}

func (c *cache) GetPairs(ctx context.Context) ([]*models.Pair, error) {
	// TODO
	return c.db.GetPairs(ctx)
}

func (c *cache) GetPair(ctx context.Context, address string) (*models.Pair, error) {
	// TODO
	return c.db.GetPair(ctx, address)
}

func (c *cache) GetTokens(ctx context.Context) ([]*models.Token, error) {
	// TODO
	return c.db.GetTokens(ctx)
}

func (c *cache) GetToken(ctx context.Context, address string) (*models.Token, error) {
	// TODO
	return c.db.GetToken(ctx, address)
}

func (c *cache) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	// TODO
	return c.db.GetTotals(ctx, from, to, interval)
}

func (c *cache) GetVolumeByPair(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	// TODO
	return c.db.GetVolumeByPair(ctx, pair, from, to, interval)
}

func (c *cache) GetLiquidityByPair(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairLiquidity, error) {
	// TODO
	return c.db.GetLiquidityByPair(ctx, pair, from, to, interval)
}

func (c *cache) GetVolumeByToken(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	// TODO
	return c.db.GetVolumeByToken(ctx, token, from, to, interval)
}

func (c *cache) GetLiquidityByToken(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenLiquidity, error) {
	// TODO
	return c.db.GetLiquidityByToken(ctx, token, from, to, interval)
}
