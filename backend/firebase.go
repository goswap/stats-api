package backend

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/goswap/stats-api/models"
	"github.com/treeder/gotils"
	"google.golang.org/api/iterator"
)

// TODO we probably want to set a max of data to return? or do something to prevent returning 3 years at 1 second, for example.

const (
	CollectionTimestamps = "timestamps"

	CollectionPairs  = "pairs"
	CollectionTokens = "tokens"

	CollectionPairBuckets  = "pair_buckets"
	CollectionTokenBuckets = "token_buckets"

	CollectionTotals = "totals" // TODO: change the name of this to just totals?  or split liquidity into separate collection?

)

type FirestoreBackend struct {
	c *firestore.Client
}

func NewFirestore(ctx context.Context, c *firestore.Client) (*FirestoreBackend, error) {
	return &FirestoreBackend{c: c}, nil
}

// GetPairs returns all available pairs
func (fs *FirestoreBackend) GetPairs(ctx context.Context) ([]*models.Pair, error) {
	pairs := make([]*models.Pair, 0)
	iter := fs.c.Collection(CollectionPairs).
		OrderBy("index", firestore.Asc).
		Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		p := new(models.Pair)
		err = doc.DataTo(p)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		p.AfterLoad(ctx)

		pairs = append(pairs, p)
	}
	return pairs, nil
}

// GetPair returns pair by address
func (fs *FirestoreBackend) GetPair(ctx context.Context, address string) (*models.Pair, error) {
	// could/should just get using doc key
	iter := fs.c.Collection(CollectionPairs).Where("address", "==", address).
		Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := &models.Pair{}
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		return t, nil
	}
	return nil, gotils.ErrNotFound
}

// GetPairByName returns pair by name
func (fs *FirestoreBackend) GetPairByName(ctx context.Context, name string) (*models.Pair, error) {
	iter := fs.c.Collection(CollectionPairs).Where("pair", "==", name).
		Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := &models.Pair{}
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		return t, nil
	}
	return nil, gotils.ErrNotFound
}

// GetTokens returns all the available tokens
func (fs *FirestoreBackend) GetTokens(ctx context.Context) ([]*models.Token, error) {
	tokens := make([]*models.Token, 0)
	iter := fs.c.Collection(CollectionTokens).
		Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := new(models.Token)
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// GetToken returns token by address
func (fs *FirestoreBackend) GetToken(ctx context.Context, address string) (*models.Token, error) {
	iter := fs.c.Collection(CollectionTokens).Where("address", "==", address).
		// OrderBy("time", firestore.Asc).
		Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := &models.Token{}
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		return t, nil
	}
	return nil, gotils.ErrNotFound
}

// GetTotals returns the total volume and liquidity over all tokens in the
// given time window at the given duration (eg per minute, per day, etc).
func (fs *FirestoreBackend) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	var totals []*models.TotalBucket

	c := fs.c.Collection(CollectionTotals)
	q := c.Query
	if !to.IsZero() {
		q = q.Where("time", "<", to)
	}
	if !from.IsZero() {
		q = q.Where("time", ">", from)
	}
	iter := q.OrderBy("time", firestore.Asc).Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := new(models.TotalBucket)
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		totals = append(totals, t)
	}

	// TODO this should be removed for pulling from aggregated data at given intervals
	// we have to go backwards to sum, to align windows for now, but still insert in chronological order
	var ie *models.TotalBucket
	var originalEnd time.Time
	ret := make([]*models.TotalBucket, 0) // hard to size correctly, default []
	for i := len(totals) - 1; i >= 0; i-- {
		t := totals[i]
		if ie == nil {
			ie = t
			if !to.IsZero() {
				ie.Time = to // overwrite this to stay in bounds of range
				originalEnd = t.Time
			}
		} else if ie.Time.Sub(t.Time) >= interval {
			// insert, then shift the window
			if ie.Time == to {
				ie.Time = originalEnd
			}
			ret = append([]*models.TotalBucket{ie}, ret...)
			ie = t
		} else {
			// add volume
			ie.VolumeUSD = ie.VolumeUSD.Add(t.VolumeUSD)
			// liquidity is just the last data point in any hour (don't add)
			// ie.LiquidityUSD = t.LiquidityUSD
		}
	}

	if ie != nil {
		if ie.Time == to {
			ie.Time = originalEnd
		}
		ret = append([]*models.TotalBucket{ie}, ret...)
	}

	return ret, nil
}

func (fs *FirestoreBackend) GetPairBuckets(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	c := fs.c.Collection(CollectionPairBuckets)
	q := c.Query
	if pair != "" {
		q = q.Where("address", "==", pair)
	}
	if !to.IsZero() {
		q = q.Where("time", "<", to)
	}
	if !from.IsZero() {
		q = q.Where("time", ">", from)
	}
	iter := q.OrderBy("time", firestore.Asc).Documents(ctx)
	defer iter.Stop()

	// in practice, the list is n=1, flexible... (TODO: weird)
	pbs := make(map[string][]*models.PairBucket)

	var n int
	for ; ; n++ {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		p := new(models.PairBucket)
		err = doc.DataTo(p)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		p.AfterLoad(ctx)
		pbs[p.Address] = append(pbs[p.Address], p)
	}

	// we want to return empty list and not null + size here
	pairs := make([]*models.PairBucket, 0, n)

	// TODO this should be removed for pulling from aggregated data at given intervals
	// we have to go backwards to sum, to align windows for now, but still insert in chronological order
	for _, pair := range pbs {
		var ie *models.PairBucket
		var originalEnd time.Time
		for i := len(pair) - 1; i >= 0; i-- {
			p := pair[i]
			if ie == nil {
				ie = p
				if !to.IsZero() {
					ie.Time = to // overwrite this to stay in bounds of range
					originalEnd = p.Time
				}
			} else if ie.Time.Sub(p.Time) >= interval {
				// insert, then shift the window
				if ie.Time == to {
					ie.Time = originalEnd
				}
				pairs = append([]*models.PairBucket{ie}, pairs...)
				ie = p
			} else {
				// add volume stuff
				ie.Amount0In = ie.Amount0In.Add(p.Amount0In)
				ie.Amount1In = ie.Amount1In.Add(p.Amount1In)
				ie.Amount0Out = ie.Amount0Out.Add(p.Amount0Out)
				ie.Amount1Out = ie.Amount1Out.Add(p.Amount1Out)
				ie.VolumeUSD = ie.VolumeUSD.Add(p.VolumeUSD)

				// liquidity/price is just the last data point in any hour (don't add)
				//ie.Price0USD = p.Price0USD
				//ie.Price1USD = p.Price1USD
				//ie.TotalSupply = p.TotalSupply
				//ie.Reserve0 = p.Reserve0
				//ie.Reserve1 = p.Reserve1
				//ie.LiquidityUSD = p.LiquidityUSD
			}
		}

		if ie != nil {
			if ie.Time == to {
				ie.Time = originalEnd
			}
			pairs = append([]*models.PairBucket{ie}, pairs...)
		}
	}

	return pairs, nil
}

func (fs *FirestoreBackend) GetTokenBuckets(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	c := fs.c.Collection(CollectionTokenBuckets)
	q := c.Query
	if token != "" {
		q = q.Where("address", "==", token)
	}
	if !to.IsZero() {
		q = q.Where("time", "<", to)
	}
	if !from.IsZero() {
		q = q.Where("time", ">", from)
	}
	iter := q.OrderBy("time", firestore.Asc).Documents(ctx)
	defer iter.Stop()

	// in practice, the list is n=1, flexible... (TODO: weird)
	tbs := make(map[string][]*models.TokenBucket)

	var n int
	for ; ; n++ {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := new(models.TokenBucket)
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		tbs[t.Address] = append(tbs[t.Address], t)
	}

	// want to default to empty list, but also size
	tokens := make([]*models.TokenBucket, 0, n)

	// TODO this should be removed for pulling from aggregated data at given intervals
	// we have to go backwards to sum, to align windows for now, but still insert in chronological order
	for _, tok := range tbs {
		var ie *models.TokenBucket
		var originalEnd time.Time
		for i := len(tok) - 1; i >= 0; i-- {
			t := tok[i]
			if ie == nil {
				ie = t
				if !to.IsZero() {
					ie.Time = to // overwrite this to stay in bounds of range
					originalEnd = t.Time
				}
			} else if ie.Time.Sub(t.Time) >= interval {
				// insert, then shift the window
				if ie.Time == to {
					ie.Time = originalEnd
				}
				tokens = append([]*models.TokenBucket{ie}, tokens...)
				ie = t
			} else {
				// add volume stuff
				ie.AmountIn = ie.AmountIn.Add(t.AmountIn)
				ie.AmountOut = ie.AmountOut.Add(t.AmountOut)
				ie.VolumeUSD = ie.VolumeUSD.Add(t.VolumeUSD)

				// dont' add these
				//ie.PriceUSD = t.PriceUSD
				//ie.Reserve = t.Reserve
				//ie.LiquidityUSD = t.LiquidityUSD
			}
		}

		if ie != nil {
			if ie.Time == to {
				ie.Time = originalEnd
			}
			tokens = append([]*models.TokenBucket{ie}, tokens...)
		}
	}

	return tokens, nil
}
