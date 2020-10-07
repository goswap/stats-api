package main

import (
	"context"
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
	"github.com/treeder/firetils"
	"github.com/treeder/gcputils"
	"github.com/treeder/goapibase"
	"github.com/treeder/gotils"
)

const (
	// DefaultTimeFrame is the default time frame
	DefaultTimeFrame = 24 * time.Hour
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
	cache, err := backend.NewCacheBackend(ctx, dbfs, 60*time.Minute)
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
	r.Route("/v1/tokens", func(r chi.Router) {
		r.Get("/", errorHandler(getTokens))
		r.Route("/{address}", func(r chi.Router) {
			r.Get("/", errorHandler(getToken))
		})
	})
	r.Route("/v1/pairs", func(r chi.Router) {
		r.Get("/", errorHandler(getPairs))

		r.Route("/{address}", func(r chi.Router) {
			r.Get("/", errorHandler(getPair))
		})
	})
	r.Route("/v1/stats", func(r chi.Router) {
		r.Get("/", errorHandler(getTotals))

		r.Route("/tokens", func(r chi.Router) {
			r.Get("/", errorHandler(getTokensStats))
			r.Route("/{address}", func(r chi.Router) {
				r.Get("/", errorHandler(getTokenBuckets))
			})
		})

		r.Route("/pairs", func(r chi.Router) {
			r.Get("/", errorHandler(getPairsStats))
			r.Route("/{address}", func(r chi.Router) {
				r.Get("/", errorHandler(getPairBuckets))
			})
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

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"tokens": ret,
	})
	return nil
}

// parseTimes parses times from query params or inserts a default
// TODO(reed): make these required fields
func parseTimes(r *http.Request) (start, end time.Time, frame time.Duration) {
	end, _ = time.Parse(time.RFC3339, r.URL.Query().Get("time_end"))
	if end.IsZero() {
		end = time.Now()
	}
	// TODO(reed): start time of 0 default won't scale well in a year?
	start, _ = time.Parse(time.RFC3339, r.URL.Query().Get("time_start"))
	// TODO(reed): reconsider, weird if end provided w/ invalid start, can still get bad frame
	//if start.IsZero() || end.Before(start) {
	//start = time.Now().Add(-24 * time.Hour)
	//}

	frame, _ = time.ParseDuration(r.URL.Query().Get("time_frame"))
	if frame == 0 {
		frame = DefaultTimeFrame
	}
	return start, end, frame
}

// returns all token sums
func getTokensStats(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	timeStart, timeEnd, _ := parseTimes(r)
	// set timeFrame to get sums
	timeFrame := timeEnd.Sub(timeStart)

	stats, err := db.GetTokenBuckets(ctx, "", timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	// TODO(reed): query parameter for volume/liquidity sorting
	sort.Slice(stats, func(i, j int) bool {
		return !stats[i].LiquidityUSD.LessThan(stats[j].LiquidityUSD)
	})

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
	})

	return nil
}

// returns a single token
func getToken(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	symbol := chi.URLParam(r, "address")
	ret, err := db.GetToken(ctx, symbol)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"token": ret,
	})
	return nil
}

func getTokenStats(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, timeFrame := parseTimes(r)
	address := chi.URLParam(r, "address")

	stats, err := db.GetTokenBuckets(ctx, address, timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
	})
	return nil
}

// returns a single pair
func getPair(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	pair := chi.URLParam(r, "address")
	ret, err := db.GetPair(ctx, pair)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"pair": ret,
	})
	return nil
}

// returns a list of all pairs
func getPairs(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	ret, err := db.GetPairs(ctx)
	if err != nil {
		return err
	}
	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"pair": ret,
	})
	return nil
}

func getPairsStats(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, _ := parseTimes(r)
	// set timeFrame to get sums
	timeFrame := timeEnd.Sub(timeStart)

	stats, err := db.GetPairBuckets(ctx, "", timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	// TODO(reed): query parameter for volume/liquidity sorting
	sort.Slice(stats, func(i, j int) bool {
		return !stats[i].LiquidityUSD.LessThan(stats[j].LiquidityUSD)
	})

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
	})
	return nil
}

func getTotals(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, timeFrame := parseTimes(r)

	totals, err := db.GetTotals(ctx, timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"stats": totals, // this has volume and liquidity
	})
	return nil
}

func getPairBuckets(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, timeFrame := parseTimes(r)
	symbol := chi.URLParam(r, "address")

	pairs, err := db.GetPairBuckets(ctx, symbol, timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"buckets": pairs,
	})
	return nil
}

func getTokenBuckets(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, timeFrame := parseTimes(r)
	symbol := chi.URLParam(r, "address")

	tokens, err := db.GetTokenBuckets(ctx, symbol, timeStart, timeEnd, timeFrame)
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
