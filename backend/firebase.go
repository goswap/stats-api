package backend

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/goswap/collector"
	"github.com/goswap/collector/models"
	"github.com/treeder/firetils"
	"github.com/treeder/gotils"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// TODO we need to normalize the times probably? ie fix dirty data
// TODO roll up into interval here?
// TODO we probably want to set a max of data to return? or do something to prevent returning 3 years at 1 second, for example.

type FirestoreBackend struct {
	c *firestore.Client
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

	return &FirestoreBackend{c: fs}, nil
}

// GetTotals returns the total volume and liquidity over all tokens in the
// given time window at the given duration (eg per minute, per day, etc).
func (fs *FirestoreBackend) GetTotals(ctx context.Context, from, to time.Time, interval time.Duration) ([]*models.TotalBucket, error) {
	var totals []*models.TotalBucket

	iter := fs.c.Collection(collector.CollectionTotalVolume).
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

	iter := fs.c.Collection(collector.CollectionPairVolume).
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

	iter := fs.c.Collection(collector.CollectionPairLiquidity).
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

	iter := fs.c.Collection(collector.CollectionTokenVolume).
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

	iter := fs.c.Collection(collector.CollectionTokenLiquidity).
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
