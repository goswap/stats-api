package backend

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/goswap/stats-api/models"
)

type mock struct {
	pairs        []*models.Pair
	tokens       []*models.Token
	pairBuckets  []*models.PairBucket
	tokenBuckets []*models.TokenBucket
	totalBuckets []*models.TotalBucket
}

// NewMock returns a mock database, for use in testing
func NewMock(args ...interface{}) StatsBackend {
	m := new(mock)
	for _, arg := range args {
		switch arg := arg.(type) {
		case []*models.Pair:
			m.pairs = arg
		case []*models.Token:
			m.tokens = arg
		case []*models.PairBucket:
			// sort by time, like we use in db
			sort.Slice(arg, func(i, j int) bool {
				return arg[i].Time.Before(arg[j].Time)
			})
			m.pairBuckets = arg
		case []*models.TokenBucket:
			// sort by time, like we use in db
			sort.Slice(arg, func(i, j int) bool {
				return arg[i].Time.Before(arg[j].Time)
			})
			m.tokenBuckets = arg
		case []*models.TotalBucket:
			// sort by time, like we use in db
			sort.Slice(arg, func(i, j int) bool {
				return arg[i].Time.Before(arg[j].Time)
			})
			m.totalBuckets = arg
		}
	}
	return m
}

func (m *mock) GetPairs(ctx context.Context) ([]*models.Pair, error) {
	return m.pairs, nil
}

func (m *mock) GetPair(ctx context.Context, address string) (*models.Pair, error) {
	for _, p := range m.pairs {
		if p.Address.Hex() == address {
			return p, nil
		}
	}
	return nil, errors.New("TODO: pair not found error")
}

func (m *mock) GetTokens(ctx context.Context) ([]*models.Token, error) {
	return m.tokens, nil
}

func (m *mock) GetToken(ctx context.Context, address string) (*models.Token, error) {
	for _, t := range m.tokens {
		if t.Address.Hex() == address {
			return t, nil
		}
	}
	return nil, errors.New("TODO: token not found error")
}

func (m *mock) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	var totals []*models.TotalBucket
	var ie *models.TotalBucket
	for _, t := range m.totalBuckets {
		if t.Time.Before(from) || to.Before(t.Time) {
			continue
		}

		// sum, if applicable
		if ie == nil {
			ie = t
		} else if t.Time.Sub(ie.Time) >= interval {
			totals = append(totals, ie)
			ie = t
		} else {
			// add volume
			ie.VolumeUSD = ie.VolumeUSD.Add(t.VolumeUSD)
			// liquidity is just the last data point in any hour (don't add)
			ie.LiquidityUSD = t.LiquidityUSD
		}
	}

	if (ie != nil && len(totals) == 0) ||
		(len(totals) > 0 && ie.Time != totals[len(totals)-1].Time) {
		totals = append(totals, ie)
	}

	return totals, nil
}

func (m *mock) GetPairBuckets(ctx context.Context, address string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	pbs := make(map[string][]*models.PairBucket)
	for _, p := range m.pairBuckets {
		if (address != "" && address != p.Address) || p.Time.Before(from) || to.Before(p.Time) {
			continue
		}

		var ie *models.PairBucket
		cp := pbs[p.Address]
		if len(cp) > 0 {
			ie = cp[len(cp)-1]
		}

		// TODO(reed): for now, we're rolling these up just before returning not in db yet
		// roll up, if required
		// TODO(reed): clean this up, it's confusing atm
		if ie == nil || p.Time.Sub(ie.Time) >= interval {
			// shift the window
			pbs[p.Address] = append(cp, p)
		} else {
			// add volume stuff
			ie.Amount0In = ie.Amount0In.Add(p.Amount0In)
			ie.Amount1In = ie.Amount1In.Add(p.Amount1In)
			ie.Amount0Out = ie.Amount0Out.Add(p.Amount0Out)
			ie.Amount1Out = ie.Amount1Out.Add(p.Amount1Out)
			ie.VolumeUSD = ie.VolumeUSD.Add(p.VolumeUSD)

			// liquidity/price is just the last data point in any hour (don't add)
			ie.Price0USD = p.Price0USD
			ie.Price1USD = p.Price1USD
			ie.TotalSupply = p.TotalSupply
			ie.Reserve0 = p.Reserve0
			ie.Reserve1 = p.Reserve1
			ie.LiquidityUSD = p.LiquidityUSD
		}
	}

	pairs := make([]*models.PairBucket, 0, len(pbs))
	for _, v := range pbs {
		pairs = append(pairs, v...)
	}
	return pairs, nil
}

func (m *mock) GetTokenBuckets(ctx context.Context, address string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	tbs := make(map[string][]*models.TokenBucket)
	for _, t := range m.tokenBuckets {
		if (address != "" && address != t.Address) || t.Time.Before(from) || to.Before(t.Time) {
			continue
		}

		var ie *models.TokenBucket
		ct := tbs[t.Address]
		if len(ct) > 0 {
			ie = ct[len(ct)-1]
		}

		// TODO(reed): for now, we're rolling these up just before returning not in db yet
		// roll up, if required
		if ie == nil || t.Time.Sub(ie.Time) >= interval {
			// shift the window
			tbs[t.Address] = append(ct, t)
		} else {
			// add volume stuff
			ie.AmountIn = ie.AmountIn.Add(t.AmountIn)
			ie.AmountOut = ie.AmountOut.Add(t.AmountOut)
			ie.VolumeUSD = ie.VolumeUSD.Add(t.VolumeUSD)

			// dont' add these
			ie.PriceUSD = t.PriceUSD
			ie.Reserve = t.Reserve
			ie.LiquidityUSD = t.LiquidityUSD
		}
	}

	tokens := make([]*models.TokenBucket, 0, len(tbs))
	for _, v := range tbs {
		tokens = append(tokens, v...)
	}

	return tokens, nil
}
