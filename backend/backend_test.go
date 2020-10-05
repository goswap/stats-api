package backend

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/goswap/stats-api/models"
	"github.com/shopspring/decimal"
)

func TestTokenBuckets(t *testing.T) {
	start := time.Now()

	zero := decimal.NewFromFloat(0)
	one := decimal.NewFromFloat(1)
	two := decimal.NewFromFloat(2)
	//three := decimal.NewFromFloat(3)
	four := decimal.NewFromFloat(4)

	seed := []*models.TokenBucket{
		{
			Address:      "0x0",
			Time:         start,
			Symbol:       "42",
			AmountIn:     one,
			AmountOut:    zero,
			PriceUSD:     one,
			VolumeUSD:    one,
			Reserve:      one,
			LiquidityUSD: one,
		},
		{
			Address:      "0x0",
			Time:         start.Add(1 * time.Hour),
			Symbol:       "42",
			AmountIn:     one,
			AmountOut:    zero,
			PriceUSD:     one,
			VolumeUSD:    one,
			Reserve:      two,
			LiquidityUSD: two,
		},
		{
			Address:      "0x1",
			Time:         start.Add(2 * time.Hour),
			Symbol:       "420",
			AmountIn:     zero,
			AmountOut:    two,
			PriceUSD:     two,
			VolumeUSD:    four,
			Reserve:      one,
			LiquidityUSD: two,
		},
	}

	ctx := context.Background()
	db := NewMock(seed)

	tests := []struct {
		addr     string
		from, to time.Time
		frame    time.Duration

		err error
		exp []*models.TokenBucket
	}{
		{"0x0", start, start.Add(59 * time.Minute), 0, nil, seed[:1]},
		{"0x0", start, start.Add(61 * time.Minute), 0, nil, seed[:2]},
		{"0x1", start, start.Add(24 * time.Hour), 0, nil, seed[2:3]},
	}

	for i, test := range tests {
		tb, err := db.GetTokenBuckets(ctx, test.addr, test.from, test.to, test.frame)
		if err != test.err {
			t.Errorf("test %v | error mismatch:\nexpected: %v\ngot: %v", i, test.err, err)
			continue
		}

		if !reflect.DeepEqual(tb, test.exp) {
			t.Errorf("test %v | results mismatch:\nexpected: %v\ngot: %v", i, test.exp, tb)
		}
	}
}
