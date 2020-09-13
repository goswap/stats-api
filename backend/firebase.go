package backend

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/dgraph-io/ristretto"
	"github.com/goswap/stats-api/models"
	"github.com/treeder/firetils"
	"github.com/treeder/gotils"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// TODO we need to normalize the times probably? ie fix dirty data
// TODO roll up into interval here?
// TODO we probably want to set a max of data to return? or do something to prevent returning 3 years at 1 second, for example.

const (
	CollectionTimestamps = "timestamps"

	CollectionPairs  = "pairs"
	CollectionTokens = "tokens"

	CollectionPairLiquidity  = "pair_liquidity"
	CollectionTokenLiquidity = "token_liquidity"

	CollectionPairVolume  = "pair_volume"
	CollectionTokenVolume = "token_volume"
	CollectionTotalVolume = "total_volume" // TODO: change the name of this to just totals?  or split liquidity into separate collection?

)

type FirestoreBackend struct {
	c     *firestore.Client
	cache *ristretto.Cache
}

func NewFirestore(ctx context.Context, projectID string, opts []option.ClientOption) (*FirestoreBackend, error) {
	firebaseApp, err := firetils.New(ctx, projectID, opts)
	if err != nil {
		return nil, err
	}
	fs, err := firebaseApp.Firestore(ctx)
	if err != nil {
		return nil, err
	}

	return NewFirestore2(ctx, fs)
}
func NewFirestore2(ctx context.Context, c *firestore.Client) (*FirestoreBackend, error) {

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
	t1, err := fs.GetToken(ctx, p.Token0Address)
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
		fs.cache.Set(address, t, 0)
		return t, nil
	}
	return nil, gotils.ErrNotFound
}

// GetTotals returns the total volume and liquidity over all tokens in the
// given time window at the given duration (eg per minute, per day, etc).
func (fs *FirestoreBackend) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	var totals []*models.TotalBucket

	iter := fs.c.Collection(CollectionTotalVolume).
		Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

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

	return totals, nil
}

// GetVolumeByPair returns the total volume by pair in the given time window
// at the given duration (eg per minute, per day, etc).
func (fs *FirestoreBackend) GetVolumeByPair(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairBucket, error) {
	var pairs []*models.PairBucket

	iter := fs.c.Collection(CollectionPairVolume).
		Where("pair", "==", pair).
		Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

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
		pairs = append(pairs, p)
	}

	// TODO: handle pair not found or bad pair input (validating backend wrapper for testing...)

	return pairs, nil
}

// GetLiquidityByPair returns the total liquidity by pair in the given time window
// at the given duration (eg per minute, per day, etc).
func (fs *FirestoreBackend) GetLiquidityByPair(ctx context.Context, pair string, from, to time.Time, interval time.Duration) ([]*models.PairLiquidity, error) {
	var pairs []*models.PairLiquidity

	iter := fs.c.Collection(CollectionPairLiquidity).
		Where("pair", "==", pair).
		Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		p := new(models.PairLiquidity)
		err = doc.DataTo(p)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		p.AfterLoad(ctx)
		pairs = append(pairs, p)
	}

	return pairs, nil
}

// GetVolumesByToken returns the total volume by token across all its pairs
// in the given time window at the given duration (eg per minute, per day,
// etc).
func (fs *FirestoreBackend) GetVolumeByToken(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenBucket, error) {
	var tokens []*models.TokenBucket

	iter := fs.c.Collection(CollectionTokenVolume).
		Where("symbol", "==", token).
		Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

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
		tokens = append(tokens, t)
	}

	return tokens, nil
}

// GetLiquidityByToken returns the total liquidity by token across all its pairs
// in the given time window at the given duration (eg per minute, per day,
// etc).
// TODO this one isn't hooked up in the collector yet? []TokenLiquidity?
func (fs *FirestoreBackend) GetLiquidityByToken(ctx context.Context, token string, from, to time.Time, interval time.Duration) ([]*models.TokenLiquidity, error) {
	// TODO
	var tokens []*models.TokenLiquidity

	iter := fs.c.Collection(CollectionTokenLiquidity).
		Where("symbol", "==", token).
		Where("time", ">", from).
		Where("time", "<", to).
		OrderBy("time", firestore.Asc).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		t := new(models.TokenLiquidity)
		err = doc.DataTo(t)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		tokens = append(tokens, t)
	}

	return tokens, nil
}
