package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/go-chi/chi"
	"github.com/goswap/stats-api/backend"
	"github.com/shopspring/decimal"
	"github.com/treeder/gcputils"
	"github.com/treeder/goapibase"
	"github.com/treeder/gotils"
)

var (
	db    backend.StatsBackend
	cache *ristretto.Cache
)

func main() {
	ctx := context.Background()

	acc, opts, err := gcputils.AccountAndCredentialsFromEnv("G_KEY")
	if err != nil {
		log.Fatal(err)
	}

	db, err = backend.NewFirestore(ctx, acc.ProjectID, opts)
	if err != nil {
		log.Fatalf("couldn't init firebase: %v\n", err)
	}

	// Setup logging, optional, typically will work fine without this, but depends on GCP service you're using
	// gcputils.InitLogging()

	// load up and cache top tokens and pairse
	// pairs, err := db.GetPairs(ctx)
	// if err != nil {
	// 	log.Fatalf("error on GetPairs: %v\n", err)
	// }

	// db.GetTop

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
			r.Get("/liquidity", errorHandler(getTokenLiquidity))
			r.Get("/volume", errorHandler(getTokenVolume))
		})
	})
	r.Route("/pairs", func(r chi.Router) {
		r.Get("/", errorHandler(getPairs))
		r.Route("/{pair}", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/volume", errorHandler(getPairVolume))
			r.Get("/liquidity", errorHandler(getPairLiquidity))
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

	liquidities := make(map[string]decimal.Decimal, len(ret))
	volumes := make(map[string]decimal.Decimal, len(ret))

	// get past 24 hours at 1 hour intervals
	to := time.Now()
	from := time.Now().Add(-24 * time.Hour)
	interval := 1 * time.Hour

	for _, r := range ret {
		// TODO: we could parallelize this but should be cached most requests sooo
		a := r.Address.Hex() // TODO hex?
		liqs, err := db.GetLiquidityByToken(ctx, a, from, to, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting liquidity for token: %v %v", a, err)
			continue
		}

		var sum decimal.Decimal
		for _, l := range liqs {
			sum.Add(l.Reserve) // TODO * price?
		}
		liquidities[a] = sum

		vols, err := db.GetVolumeByToken(ctx, a, from, to, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting volume for token: %v %v", a, err)
			continue
		}

		var sum2 decimal.Decimal
		for _, v := range vols {
			sum2.Add(v.VolumeUSD) // TODO * price?
		}
		volumes[a] = sum2
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"tokens":      ret,
		"liquidities": liquidities,
		"volumes":     volumes,
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

	liquidities := make(map[string]decimal.Decimal, len(ret))
	volumes := make(map[string]decimal.Decimal, len(ret))

	// get past 24 hours at 1 hour intervals
	to := time.Now()
	from := time.Now().Add(-24 * time.Hour)
	interval := 1 * time.Hour

	for _, r := range ret {
		// TODO: we could parallelize this but should be cached most requests sooo
		a := r.Address.Hex() // TODO hex?
		liqs, err := db.GetLiquidityByPair(ctx, a, from, to, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting liquidity for pair: %v %v", a, err)
			log.Println(err) // TODO remove
			continue
		}

		var sum decimal.Decimal
		for _, l := range liqs {
			sum.Add(l.TotalSupply) // TODO * price?
		}
		liquidities[a] = sum

		vols, err := db.GetVolumeByPair(ctx, a, from, to, interval)
		if err != nil {
			// TODO log and move on
			gcputils.Error().Printf("error getting volume for pair: %v %v", a, err)
			log.Println(err)
			continue
		}

		var sum2 decimal.Decimal
		for _, v := range vols {
			sum2.Add(v.VolumeUSD) // TODO * price?
		}
		volumes[a] = sum2
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"pairs":       ret,
		"liquidities": liquidities,
		"volumes":     volumes,
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

func getPairVolume(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)
	symbol := chi.URLParam(r, "pair")

	pairs, err := db.GetVolumeByPair(ctx, symbol, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": pairs,
	})
	return nil
}

func getPairLiquidity(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)
	symbol := chi.URLParam(r, "pair")

	pairs, err := db.GetLiquidityByPair(ctx, symbol, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": pairs,
	})
	return nil
}

func getTokenVolume(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)
	symbol := chi.URLParam(r, "symbol")

	tokens, err := db.GetVolumeByToken(ctx, symbol, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": tokens,
	})
	return nil
}

func getTokenLiquidity(w http.ResponseWriter, r *http.Request) error {
	// TODO query parameters for times, interval
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	interval := time.Duration(0)
	symbol := chi.URLParam(r, "symbol")

	tokens, err := db.GetLiquidityByToken(ctx, symbol, timeStart, timeEnd, interval)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": tokens,
	})
	return nil
}

func collect(w http.ResponseWriter, r *http.Request) error {
	// prevent this from running more than once per hour
	gcputils.Info().Println("testing...")
	gotils.WriteMessage(w, http.StatusOK, "hi")
	return nil
}
