package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi"
	"github.com/goswap/collector/models"
	"github.com/treeder/firetils"
	"github.com/treeder/gcputils"
	"github.com/treeder/goapibase"
	"github.com/treeder/gotils"
	"google.golang.org/api/iterator"
)

const (
	CollectionTimestamps  = "timestamps"
	CollectionPairVolume  = "pair_volume"
	CollectionTokenVolume = "token_volume"
	CollectionTotalVolume = "total_volume"
)

var (
	fs *firestore.Client
)

func main() {
	ctx := context.Background()

	acc, opts, err := gcputils.AccountAndCredentialsFromEnv("G_KEY")
	if err != nil {
		log.Fatal(err)
	}

	// Setup logging, optional, typically will work fine without this, but depends on GCP service you're using
	// gcputils.InitLogging()

	firebaseApp, err := firetils.New(ctx, acc.ProjectID, opts)
	if err != nil {
		gotils.L(ctx).Sugar().Fatalf("couldn't init firebase newapp: %v\n", err)
	}
	fs, err = firebaseApp.Firestore(ctx)
	if err != nil {
		gotils.L(ctx).Sugar().Fatalf("couldn't init firestore: %v\n", err)
	}

	r := goapibase.InitRouter(ctx)
	// Setup your routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})
	r.Route("/tokens", func(r chi.Router) {
		r.Route("/{symbol}", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/", getToken) // GET /articles/123
		})
	})
	r.Route("/pairs", func(r chi.Router) {
		r.Route("/{pair}", func(r chi.Router) {
			// r.Use(ArticleCtx)
			r.Get("/", getToken) // GET /articles/123
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
type MyHandlerFunc func(w http.ResponseWriter, r *http.Request) error

func errorHandler(h MyHandlerFunc) http.HandlerFunc {
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

func getTotals(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	timeStart := time.Now().AddDate(0, 0, -1)
	timeEnd := time.Now()
	totals := []*models.TotalBucket{}
	iter := fs.Collection(CollectionTotalVolume).Where("time", ">", timeStart).Where("time", "<", timeEnd).OrderBy("time", firestore.Asc).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return gotils.C(ctx).Errorf("error getting data: %v", err)
		}
		// fmt.Println(doc.Data())
		t := &models.TotalBucket{}
		err = doc.DataTo(t)
		if err != nil {
			return gotils.C(ctx).Errorf("%v", err)
		}
		t.AfterLoad(ctx)
		totals = append(totals, t)
	}
	gotils.WriteObject(w, http.StatusOK, map[string]interface{}{
		"overTime": totals, // this has volume and liquidity
	})
	return nil
}

func getToken(w http.ResponseWriter, r *http.Request) {
	// ctx := r.Context()
	symbol := chi.URLParam(r, "symbol")
	// fs.Collection(CollectionTokenVolume).Where()
	w.Write([]byte(fmt.Sprintf("title:%s", symbol)))
}
