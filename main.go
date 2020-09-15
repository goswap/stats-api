package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/goswap/stats-api/backend"
	"github.com/goswap/stats-api/models"
	"github.com/treeder/gcputils"
	"github.com/treeder/goapibase"
	"github.com/treeder/gotils"
)

var (
	db backend.StatsBackend
)

func main() {
	ctx := context.Background()

	acc, opts, err := gcputils.AccountAndCredentialsFromEnv("G_KEY")
	if err != nil {
		log.Fatal(err)
	}

	dbfs, err := backend.NewFirestore(ctx, acc.ProjectID, opts)
	if err != nil {
		log.Fatalf("couldn't init firebase: %v\n", err)
	}
	db = dbfs

	// Setup logging, optional, typically will work fine without this, but depends on GCP service you're using
	// gcputils.InitLogging()

	// load up and cache top tokens and pairs
	// pairs, err := db.GetPairs(ctx)
	// if err != nil {
	// 	log.Fatalf("error on GetPairs: %v\n", err)
	// }

	// // TODO: the following will get heavy quickly as we add more pairs, need to change this
	// // TODO: perhaps the collector can update these values on the Pair and Token objects directly so it's just done once during collection runs.
	// // fs := dbfs.Client()
	// timeStart := time.Now().AddDate(0, 0, -1)
	// timeEnd := time.Now()
	// interval := time.Duration(0)
	// pairBuckets, err := db.GetPairBuckets(ctx, "", timeStart, timeEnd, interval)
	// if err != nil {
	// 	log.Fatalf("error on GetPairBuckets: %v\n", err)
	// }
	// tokenBuckets, err := db.GetTokenBuckets(ctx, "", timeStart, timeEnd, interval)
	// if err != nil {
	// 	log.Fatalf("error on GetTokenBuckets: %v\n", err)
	// }

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
			// r.Get("/volume", errorHandler(getTokenVolume))
		})
	})
	r.Route("/pairs", func(r chi.Router) {
		r.Get("/", errorHandler(getPairs))
		r.Route("/{pair}", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/buckets", errorHandler(getPairBuckets))
			// r.Get("/liquidity", errorHandler(getPairLiquidity))
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
			stats.Reserve = l.Reserve
			stats.PriceUSD = l.PriceUSD
			stats.LiquidityUSD = l.Reserve.Mul(l.PriceUSD)
			for _, l := range liqs {
				stats.VolumeUSD = stats.VolumeUSD.Add(l.VolumeUSD)
			}
		}
		statsMap[a] = stats
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"tokens": ret,
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
		fmt.Println("A:", a)
		liqs, err := db.GetPairBuckets(ctx, a, timeStart, timeEnd, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting liquidity for token: %v %v", a, err)
			continue
		}

		stats := &models.PairBucket{}
		if len(liqs) > 0 {
			l := liqs[len(liqs)-1]
			stats.Reserve0 = l.Reserve0
			stats.Reserve1 = l.Reserve1
			stats.Price0USD = l.Price0USD
			stats.Price1USD = l.Price1USD
			stats.TotalSupply = l.TotalSupply
			stats.LiquidityUSD = l.Reserve0.Mul(l.Price0USD).Add(l.Reserve1.Mul(l.Price1USD))
			for _, l := range liqs {
				stats.Amount0In = stats.Amount0In.Add(l.Amount0In)
				stats.Amount1In = stats.Amount1In.Add(l.Amount1In)
				stats.VolumeUSD = stats.VolumeUSD.Add(l.VolumeUSD)
			}
		}
		statsMap[a] = stats
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"pairs": ret,
		"stats": statsMap,
	})
	return nil
}

func getTotals(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)

	totals, err := db.GetTotals(ctx, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": totals, // this has volume and liquidity
	})
	return nil
}

// func getPairVolume(w http.ResponseWriter, r *http.Request) error {
// 	// TODO query parameters for times, interval
// 	ctx := r.Context()
// 	timeStart := time.Now().AddDate(0, 0, -1)
// 	timeEnd := time.Now()
// 	interval := time.Duration(0)
// 	symbol := chi.URLParam(r, "pair")

// 	pairs, err := db.GetVolumeByPair(ctx, symbol, timeStart, timeEnd, interval)
// 	if err != nil {
// 		return err
// 	}

// 	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
// 		"overTime": pairs,
// 	})
// 	return nil
// }

func getPairBuckets(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)
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

// func getTokenVolume(w http.ResponseWriter, r *http.Request) error {
// 	// TODO query parameters for times, interval
// 	ctx := r.Context()
// 	timeStart := time.Now().AddDate(0, 0, -1)
// 	timeEnd := time.Now()
// 	interval := time.Duration(0)
// 	symbol := chi.URLParam(r, "symbol")

// 	tokens, err := db.GetVolumeByToken(ctx, symbol, timeStart, timeEnd, interval)
// 	if err != nil {
// 		return err
// 	}

// 	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
// 		"overTime": tokens,
// 	})
// 	return nil
// }

func getTokenBuckets(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)
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
	// prevent this from running more than once per hour
	gcputils.Info().Println("testing...")
	gotils.WriteMessage(w, http.StatusOK, "hi")
	return nil
}
