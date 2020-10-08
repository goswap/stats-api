package main

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi"
	"github.com/gochain/gochain/v3/goclient"
	"github.com/gochain/gochain/v3/rpc"
	"github.com/goswap/stats-api/backend"
	"github.com/goswap/stats-api/collector"
	"github.com/goswap/stats-api/models"
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

	// errors
	errParamTimeRequired = gotils.NewHttpError("time_start and time_end not provided or invalid", 400)
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
func parseTimes(r *http.Request) (start, end time.Time, frame time.Duration, err error) {
	end, _ = time.Parse(time.RFC3339, r.URL.Query().Get("time_end"))
	if end.IsZero() {
		return start, end, frame, errParamTimeRequired
	}
	start, _ = time.Parse(time.RFC3339, r.URL.Query().Get("time_start"))
	if start.IsZero() {
		return start, end, frame, errParamTimeRequired
	}

	frame, _ = time.ParseDuration(r.URL.Query().Get("time_frame"))
	if frame == 0 {
		frame = DefaultTimeFrame
	}
	return start, end, frame, nil
}

func sortTokenBuckets(stats []*models.TokenBucket, key string, desc bool) {
	// this just does a simple xor, go doesn't have a nice operator for it. this could probably be
	// cleaned up, maybe to not need the closure would be nice, it's yielded from the switch
	// to avoid the switch being called in every sort call to LessThan (f)
	x := desc
	var f func(i, j int) bool
	switch key {
	case "address":
		f = func(i, j int) bool { y := stats[i].Address < stats[j].Address; return (x || y) && !(x && y) }
	case "time":
		f = func(i, j int) bool { y := stats[i].Time.Before(stats[j].Time); return (x || y) && !(x && y) }
	case "symbol":
		f = func(i, j int) bool { y := stats[i].Symbol < stats[j].Symbol; return (x || y) && !(x && y) }
	case "amountIn":
		f = func(i, j int) bool { y := stats[i].AmountIn.LessThan(stats[j].AmountIn); return (x || y) && !(x && y) }
	case "amountOut":
		f = func(i, j int) bool {
			y := stats[i].AmountOut.LessThan(stats[j].AmountOut)
			return (x || y) && !(x && y)
		}
	case "priceUSD":
		f = func(i, j int) bool { y := stats[i].PriceUSD.LessThan(stats[j].PriceUSD); return (x || y) && !(x && y) }
	case "volumeUSD":
		f = func(i, j int) bool {
			y := stats[i].VolumeUSD.LessThan(stats[j].VolumeUSD)
			return (x || y) && !(x && y)
		}
	case "reserve":
		f = func(i, j int) bool { y := stats[i].Reserve.LessThan(stats[j].Reserve); return (x || y) && !(x && y) }
	case "liquidityUSD":
		fallthrough // default
	default:
		f = func(i, j int) bool {
			y := stats[i].LiquidityUSD.LessThan(stats[j].LiquidityUSD)
			return (x || y) && !(x && y)
		}
	}

	sort.Slice(stats, f)
}

// returns all token sums
func getTokensStats(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	timeStart, timeEnd, _, err := parseTimes(r)
	if err != nil {
		return err
	}
	// set timeFrame to get sums
	timeFrame := timeEnd.Sub(timeStart)

	sortKey := r.URL.Query().Get("sort")
	if sortKey == "" {
		sortKey = "-liquidityUSD"
	}
	// slightly confusingly, if + not provided, do asc (even tho -liquidityUSD is default)
	sortDesc := sortKey == "" || strings.HasPrefix(sortKey, "-")
	if strings.HasPrefix(sortKey, "-") || strings.HasPrefix(sortKey, "+") {
		sortKey = sortKey[1:]
	}

	stats, err := db.GetTokenBuckets(ctx, "", timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	// TODO(reed): we prob can't do this here in combination with pagination, we
	// need to bound the domain for time_frame, push down to db and not use
	// client side aggregation to have rows for each time_frame in order to avoid
	// data overload? on the other hand, if we load all the data into the cache
	// and do paging / sorting of the cached data, that makes more sense? needs a
	// good think since firebase doesn't support doing sums and then paging over
	// it (we have to do them)
	sortTokenBuckets(stats, sortKey, !sortDesc)

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
	timeStart, timeEnd, timeFrame, err := parseTimes(r)
	if err != nil {
		return err
	}
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

func sortPairBuckets(stats []*models.PairBucket, key string, desc bool) {
	x := desc
	var f func(i, j int) bool
	switch key {
	case "address":
		f = func(i, j int) bool { y := stats[i].Address < stats[j].Address; return (x || y) && !(x && y) }
	case "time":
		f = func(i, j int) bool { y := stats[i].Time.Before(stats[j].Time); return (x || y) && !(x && y) }
	case "pair":
		f = func(i, j int) bool { y := stats[i].Pair < stats[j].Pair; return (x || y) && !(x && y) }
	case "amount0In":
		f = func(i, j int) bool {
			y := stats[i].Amount0In.LessThan(stats[j].Amount0In)
			return (x || y) && !(x && y)
		}
	case "amount1In":
		f = func(i, j int) bool {
			y := stats[i].Amount1In.LessThan(stats[j].Amount1In)
			return (x || y) && !(x && y)
		}
	case "amount0Out":
		f = func(i, j int) bool {
			y := stats[i].Amount0Out.LessThan(stats[j].Amount0Out)
			return (x || y) && !(x && y)
		}
	case "amount1Out":
		f = func(i, j int) bool {
			y := stats[i].Amount1Out.LessThan(stats[j].Amount1Out)
			return (x || y) && !(x && y)
		}
	case "price0USD":
		f = func(i, j int) bool {
			y := stats[i].Price0USD.LessThan(stats[j].Price0USD)
			return (x || y) && !(x && y)
		}
	case "price1USD":
		f = func(i, j int) bool {
			y := stats[i].Price1USD.LessThan(stats[j].Price1USD)
			return (x || y) && !(x && y)
		}
	case "volumeUSD":
		f = func(i, j int) bool {
			y := stats[i].VolumeUSD.LessThan(stats[j].VolumeUSD)
			return (x || y) && !(x && y)
		}
	case "reserve0":
		f = func(i, j int) bool { y := stats[i].Reserve0.LessThan(stats[j].Reserve0); return (x || y) && !(x && y) }
	case "reserve1":
		f = func(i, j int) bool { y := stats[i].Reserve1.LessThan(stats[j].Reserve1); return (x || y) && !(x && y) }
	case "liquidityUSD":
		fallthrough // default
	default:
		f = func(i, j int) bool {
			y := stats[i].LiquidityUSD.LessThan(stats[j].LiquidityUSD)
			return (x || y) && !(x && y)
		}
	}

	sort.Slice(stats, f)
}

func getPairsStats(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, _, err := parseTimes(r)
	if err != nil {
		return err
	}
	// set timeFrame to get sums
	timeFrame := timeEnd.Sub(timeStart)

	sortKey := r.URL.Query().Get("sort")
	if sortKey == "" {
		sortKey = "-liquidityUSD"
	}
	// slightly confusingly, if + not provided, do asc (even tho -liquidityUSD is default)
	sortDesc := sortKey == "" || strings.HasPrefix(sortKey, "-")
	if strings.HasPrefix(sortKey, "-") || strings.HasPrefix(sortKey, "+") {
		sortKey = sortKey[1:]
	}

	stats, err := db.GetPairBuckets(ctx, "", timeStart, timeEnd, timeFrame)
	if err != nil {
		return err
	}

	// TODO(reed): see note on sortTokenBuckets, may not be the place (eventually)
	sortPairBuckets(stats, sortKey, sortDesc)

	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
	})
	return nil
}

func getTotals(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart, timeEnd, timeFrame, err := parseTimes(r)
	if err != nil {
		return err
	}

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
	timeStart, timeEnd, timeFrame, err := parseTimes(r)
	if err != nil {
		return err
	}
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
	timeStart, timeEnd, timeFrame, err := parseTimes(r)
	if err != nil {
		return err
	}
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
