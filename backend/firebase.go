package backend

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/dgraph-io/ristretto"
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
	c     *firestore.Client
	cache *ristretto.Cache
}

func NewFirestore(ctx context.Context, c *firestore.Client) (*FirestoreBackend, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,             // number of keys to track frequency of (10M).
		MaxCost:     100 << (10 * 2), // maximum cost of cache (100MB), cloud run has 256MB by default
		BufferItems: 64,              // number of keys per Get buffer.
	})
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on NewCache: %v", err)
	}

	return &FirestoreBackend{c: c, cache: cache}, nil
}

// GetPairs returns all available pairs
func (fs *FirestoreBackend) GetPairs(ctx context.Context) ([]*models.Pair, error) {
	var pairs []*models.Pair
	iter := fs.c.Collection(CollectionPairs).
		OrderBy("index", firestore.Asc).
		Documents(ctx)

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

		err = fs.setPairTokens(ctx, p)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}

		// TODO(reed): these values will get stale, like supply, need ttl...
		fs.cache.Set(p.Address.Hex(), p, 0)
	}
	return pairs, nil
}

func (fs *FirestoreBackend) setPairTokens(ctx context.Context, p *models.Pair) error {
	// get tokens
	t0, err := fs.GetToken(ctx, p.Token0Address)
	if err != nil {
		return gotils.C(ctx).Errorf("%v", err)
	}
	p.Token0 = t0
	t1, err := fs.GetToken(ctx, p.Token1Address)
	if err != nil {
		return gotils.C(ctx).Errorf("%v", err)
	}
	p.Token1 = t1
	return nil
}

// GetPair returns pair by address
func (fs *FirestoreBackend) GetPair(ctx context.Context, address string) (*models.Pair, error) {
	v, _ := fs.cache.Get(address)
	if v != nil {
		return v.(*models.Pair), nil
	}

	// could/should just get using doc key
	iter := fs.c.Collection(CollectionPairs).Where("address", "==", address).
		// OrderBy("time", firestore.Asc).
		Documents(ctx)

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
		fs.cache.Set(address, t, 0)
		return t, nil
	}
	return nil, gotils.ErrNotFound
}

// GetTokens returns all the available tokens
func (fs *FirestoreBackend) GetTokens(ctx context.Context) ([]*models.Token, error) {
	var tokens []*models.Token
	iter := fs.c.Collection(CollectionTokens).
		// OrderBy("time", firestore.Asc).
		Documents(ctx)

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
		fs.cache.Set(t.Address.Hex(), t, 0)
	}
	return tokens, nil
}

// GetToken returns token by address
func (fs *FirestoreBackend) GetToken(ctx context.Context, address string) (*models.Token, error) {
	v, _ := fs.cache.Get(address)
	if v != nil {
		return v.(*models.Token), nil
	}

	iter := fs.c.Collection(CollectionTokens).Where("address", "==", address).
		// OrderBy("time", firestore.Asc).
		Documents(ctx)

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
		// TODO(reed): this value will get stale (volume, cmcprice)
		fs.cache.Set(address, t, 0)
		return t, nil
	}
	return nil, gotils.ErrNotFound
}

// GetTotals returns the total volume and liquidity over all tokens in the
// given time window at the given duration (eg per minute, per day, etc).
func (fs *FirestoreBackend) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	var totals []*models.TotalBucket

	iter := fs.c.Collection(CollectionTotals).
		Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

	// interval edge
	var ie *models.TotalBucket

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

		// TODO(reed): for now, we're rolling these up just before returning not in db yet
		// roll up, if required
		if ie == nil {
			ie = t
		} else if t.Time.Sub(ie.Time) >= interval {
			// put last one in, then shift the window
			totals = append(totals, ie)
			ie = t
		} else {
			// add volume
			ie.VolumeUSD = ie.VolumeUSD.Add(t.VolumeUSD)
			// liquidity is just the last data point in any hour (don't add)
			ie.LiquidityUSD = t.LiquidityUSD
		}
	}

	// insert last entry
	if ie != nil {
		totals = append(totals, ie)
	}

	return totals, nil
}

func (fs *FirestoreBackend) GetPairBuckets(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	var pairs []*models.PairBucket

	c := fs.c.Collection(CollectionPairBuckets)
	q := c.Query
	if pair != "" {
		q = q.Where("address", "==", pair)
	}
	iter := q.Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

	var ie *models.PairBucket

	for {
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

		// TODO(reed): for now, we're rolling these up just before returning not in db yet
		// roll up, if required
		if ie == nil {
			ie = p
		} else if p.Time.Sub(ie.Time) >= interval {
			// put last one in, then shift the window
			pairs = append(pairs, ie)
			ie = p
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

	if ie != nil {
		pairs = append(pairs, ie)
	}

	// TODO: handle pair not found or bad pair input (validating backend wrapper for testing...)

	return pairs, nil
}

func (fs *FirestoreBackend) GetTokenBuckets(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	var tokens []*models.TokenBucket

	c := fs.c.Collection(CollectionTokenBuckets)
	q := c.Query
	if token != "" {
		q = q.Where("address", "==", token)
	}
	iter := q.Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

	var ie *models.TokenBucket

	for {
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

		// TODO(reed): for now, we're rolling these up just before returning not in db yet
		// roll up, if required
		if ie == nil {
			ie = t
		} else if t.Time.Sub(ie.Time) >= interval {
			// put last one in, then shift the window
			tokens = append(tokens, ie)
			ie = t
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

	if ie != nil {
		tokens = append(tokens, ie)
	}

	return tokens, nil
}
