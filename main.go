package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi"
	"github.com/gochain/gochain/v3/goclient"
	"github.com/gochain/gochain/v3/rpc"
	"github.com/goswap/stats-api/backend"
	"github.com/goswap/stats-api/collector"
	"github.com/goswap/stats-api/models"
	"github.com/shopspring/decimal"
	"github.com/treeder/firetils"
	"github.com/treeder/gcputils"
	"github.com/treeder/goapibase"
	"github.com/treeder/gotils"
)

var (
	db backend.StatsBackend

	// TODO should hide this behind collector interface
	fsc *firestore.Client

	rpcURL = "https://rpc.gochain.io"
)

func main() {
	ctx := context.Background()

	acc, opts, err := gcputils.AccountAndCredentialsFromEnv("G_KEY")
	if err != nil {
		log.Fatal(err)
	}

	firebaseApp, err := firetils.New(ctx, acc.ProjectID, opts)
	if err != nil {
		log.Fatalf("couldn't create firebase app: %v\n", err)
	}
	fsc, err = firebaseApp.Firestore(ctx)
	if err != nil {
		log.Fatalf("couldn't create firebase client: %v\n", err)
	}

	dbfs, err := backend.NewFirestore(ctx, fsc)
	if err != nil {
		log.Fatalf("couldn't init firebase: %v\n", err)
	}

	// TODO we could add more fine grained ttl, this is a stand in.
	cache, err := backend.NewCacheBackend(ctx, dbfs, 1*time.Minute)
	if err != nil {
		log.Fatalf("couldn't set up cache: %v\n", err)
	}

	db = cache

	// Setup logging, optional, typically will work fine without this, but depends on GCP service you're using
	// gcputils.InitLogging()

	// TODO: we could pre-warm some of the caches, if we really want to here before starting traffic

	r := goapibase.InitRouter(ctx)
	// Setup your routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})
	r.Post("/collect", errorHandler(collect))
	r.Route("/tokens", func(r chi.Router) {
		r.Get("/", errorHandler(getTokens))
		r.Route("/{symbol}", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/buckets", errorHandler(getTokenBuckets))
		})
	})
	r.Route("/pairs", func(r chi.Router) {
		r.Get("/", errorHandler(getPairs))
		r.Route("/{pair}", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/buckets", errorHandler(getPairBuckets))
		})
	})
	r.Route("/totals", func(r chi.Router) {
		r.Route("/", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/", errorHandler(getTotals)) // GET /articles/123
		})
	})
	// Start server
	_ = goapibase.Start(ctx, gotils.Port(8080), r)
}

// todo: move this stuff to gotils
type myHandlerFunc func(w http.ResponseWriter, r *http.Request) error

func errorHandler(h myHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			switch e := err.(type) {
			case *gotils.HttpError:
				gcputils.Error().Printf("%v", err)
				gotils.WriteError(w, e.Code(), e)
			default:
				gcputils.Error().Printf("%v", err) // to cloud logging
				gotils.WriteError(w, http.StatusInternalServerError, e)
			}
		}
	}
}

// returns a list of all tokens
func getTokens(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	ret, err := db.GetTokens(ctx)
	if err != nil {
		return err
	}

	statsMap := make(map[string]*models.TokenBucket, len(ret))
	// volumes := make(map[string]decimal.Decimal, len(ret))

	// get past 24 hours at 1 hour intervals
	timeEnd := time.Now()
	timeStart := timeEnd.AddDate(0, 0, -1)
	interval := 1 * time.Hour

	for _, r := range ret {
		// TODO: we could parallelize this but should be cached most requests sooo
		a := r.Address.Hex() // TODO hex?
		liqs, err := db.GetTokenBuckets(ctx, a, timeStart, timeEnd, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting liquidity for token: %v %v", a, err)
			continue
		}

		stats := &models.TokenBucket{}
		if len(liqs) > 0 {
			l := liqs[len(liqs)-1]
			// fmt.Printf("%v LIQUIDITY %v %v\n", r.String(), l.Reserve, l.PriceUSD)
			stats.Reserve = l.Reserve
			stats.PriceUSD = l.PriceUSD
			stats.LiquidityUSD = l.Reserve.Mul(l.PriceUSD)
			// fmt.Printf("LIQUIDITY 2: %v\n", stats.LiquidityUSD)
			for _, l := range liqs {
				stats.VolumeUSD = stats.VolumeUSD.Add(l.VolumeUSD)
				// fmt.Printf("%v LIQUIDITY XXX %v %v\n", r.String(), l.Reserve, l.PriceUSD)
			}
		}
		statsMap[a] = stats
	}

	sort.Slice(ret, func(i, j int) bool {
		return !(statsMap[ret[i].AddressHex].LiquidityUSD.LessThan(statsMap[ret[j].AddressHex].LiquidityUSD))
	})

	// remove 0 liquidity
	ret2 := []*models.Token{}
	for _, r := range ret {
		if statsMap[r.Address.Hex()].LiquidityUSD.Equal(decimal.Zero) {
			continue
		}
		ret2 = append(ret2, r)
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"tokens": ret2,
		"stats":  statsMap,
	})

	return nil
}

// returns a list of all tokens
func getPairs(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	ret, err := db.GetPairs(ctx)
	if err != nil {
		return err
	}
	statsMap := make(map[string]*models.PairBucket, len(ret))
	// volumes := make(map[string]decimal.Decimal, len(ret))

	// get past 24 hours at 1 hour intervals
	timeEnd := time.Now()
	timeStart := timeEnd.AddDate(0, 0, -1)
	interval := 1 * time.Hour

	for _, r := range ret {
		// TODO: we could parallelize this but should be cached most requests sooo
		a := r.Address.Hex() // TODO hex?
		liqs, err := db.GetPairBuckets(ctx, a, timeStart, timeEnd, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting liquidity for token: %v %v", a, err)
			continue
		}

		stats := &models.PairBucket{}
		if len(liqs) > 0 {
			l := liqs[len(liqs)-1]
			fmt.Printf("%v LIQUIDITY %v %v %v %v\n", r.String(), l.Reserve0, l.Reserve1, l.Price0USD, l.Price1USD)
			stats.Reserve0 = l.Reserve0
			stats.Reserve1 = l.Reserve1
			stats.Price0USD = l.Price0USD
			stats.Price1USD = l.Price1USD
			stats.TotalSupply = l.TotalSupply
			stats.LiquidityUSD = l.Reserve0.Mul(l.Price0USD).Add(l.Reserve1.Mul(l.Price1USD))
			fmt.Printf("LIQUIDITY 2: %v\n", stats.LiquidityUSD)
			for _, l := range liqs {
				stats.Amount0In = stats.Amount0In.Add(l.Amount0In)
				stats.Amount1In = stats.Amount1In.Add(l.Amount1In)
				stats.VolumeUSD = stats.VolumeUSD.Add(l.VolumeUSD)
			}
		}
		statsMap[a] = stats
	}

	sort.Slice(ret, func(i, j int) bool {
		// Using not less to make it descending order
		return !(statsMap[ret[i].AddressHex].LiquidityUSD.LessThan(statsMap[ret[j].AddressHex].LiquidityUSD))
	})

	ret2 := []*models.Pair{}
	for _, r := range ret {
		if statsMap[r.Address.Hex()].LiquidityUSD.Equal(decimal.Zero) {
			continue
		}
		ret2 = append(ret2, r)
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"pairs": ret2,
		"stats": statsMap,
	})
	return nil
}

func getTotals(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	// TODO(reed): 0 < x < now is a harsh default, lots of data
	timeStart, _ := time.Parse(time.RFC3339, r.URL.Query().Get("start_time"))
	timeEnd, _ := time.Parse(time.RFC3339, r.URL.Query().Get("end_time"))
	if timeEnd.IsZero() {
		timeEnd = time.Now() // default to latest
	}
	interval, _ := time.ParseDuration(r.URL.Query().Get("interval"))
	// TODO we should limit interval to 1h or 24h only, default 24h?

	totals, err := db.GetTotals(ctx, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": totals, // this has volume and liquidity
	})
	return nil
}

func getPairBuckets(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	// TODO(reed): 0 < x < now is a harsh default, lots of data
	timeStart, _ := time.Parse(time.RFC3339, r.URL.Query().Get("start_time"))
	timeEnd, _ := time.Parse(time.RFC3339, r.URL.Query().Get("end_time"))
	if timeEnd.IsZero() {
		timeEnd = time.Now() // default to latest
	}
	interval, _ := time.ParseDuration(r.URL.Query().Get("interval"))
	// TODO we should limit interval to 1h or 24h only, default 24h?
	symbol := chi.URLParam(r, "pair")

	pairs, err := db.GetPairBuckets(ctx, symbol, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"buckets": pairs,
	})
	return nil
}

func getTokenBuckets(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()

	// TODO(reed): 0 < x < now is a harsh default, lots of data
	timeStart, _ := time.Parse(time.RFC3339, r.URL.Query().Get("start_time"))
	timeEnd, _ := time.Parse(time.RFC3339, r.URL.Query().Get("end_time"))
	if timeEnd.IsZero() {
		timeEnd = time.Now() // default to latest
	}
	interval, _ := time.ParseDuration(r.URL.Query().Get("interval"))
	// TODO we should limit interval to 1h or 24h only, default 24h?
	symbol := chi.URLParam(r, "symbol")

	tokens, err := db.GetTokenBuckets(ctx, symbol, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"buckets": tokens,
	})
	return nil
}

func collect(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	t := time.Now()
	ctx = gotils.With(ctx, "started_at", t)
	l := gcputils.With("started_at", t)
	// prevent this from running more than once per hour
	l.Info().Println("Collector starting...")
	rpcClient, err := rpc.Dial(rpcURL)
	if err != nil {
		return gotils.C(ctx).Errorf("failed to dial rpc %q: %v", rpcURL, err)
	}
	rpc := goclient.NewClient(rpcClient)

	err = collector.FetchData(ctx, rpc, fsc)
	if err != nil {
		return gotils.C(ctx).Errorf("error on collector.FetchData: %v", err)
	}
	l.Info().Println("Collector complete")
	gotils.WriteMessage(w, http.StatusOK, "ok")
	return nil
}
