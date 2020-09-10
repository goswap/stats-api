package backend

import (
	"context"
	"time"

	"github.com/goswap/collector/models"
)

// TODO(reed): the interval seems wise but it may be the case that we want to
// do this elsewhere (say, collector, or above this), it should be easy enough
// still to cache the entire data set for some time and roll up into higher
// interval quickly before returning.  TBD but putting here to remind even if
// unused.

// TODO(reed): if from=to then return only latest point lookup? smaller API here...

// StatsBackend defines methods for accessing goswap statistics
type StatsBackend interface {
	// GetTotals returns the total volume and liquidity over all tokens in the
	// given time window at the given duration (eg per minute, per day, etc).
	GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error)

	// GetVolumeByPair returns the total volume by pair in the given time window
	// at the given duration (eg per minute, per day, etc).
	GetVolumeByPair(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error)

	// GetLiquidityByPair returns the total liquidity by pair in the given time window
	// at the given duration (eg per minute, per day, etc).
	GetLiquidityByPair(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairLiquidity, error)

	// GetVolumesByToken returns the total volume by token across all its pairs
	// in the given time window at the given duration (eg per minute, per day,
	// etc).
	GetVolumeByToken(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error)

	// GetLiquidityByToken returns the total liquidity by token across all its pairs
	// in the given time window at the given duration (eg per minute, per day,
	// etc).
	GetLiquidityByToken(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error)

	// TODO(reed): do we want GetPriceByToken and just return USD? we can 'magic' this on front end too and
	// just use the right pair?

	// TODO(reed): do we want GetPriceByPair? it's rolled into PairLiquidity atm
}
