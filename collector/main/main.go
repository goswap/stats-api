package main

import (
	"context"
	"log"

	"github.com/gochain/gochain/v4/goclient"
	"github.com/gochain/gochain/v4/rpc"
	"github.com/goswap/stats-api/collector"
	"github.com/treeder/firetils"
	"github.com/treeder/gcputils"
	"github.com/treeder/gotils"
)

var (
	rpcURL = "https://rpc.gochain.io"
)

/*
So this script will run on a schedule and collect the following information:
1) Volume over time
2) List of all pairs with their total liquidity and current price
3) 24 hr volume on those pairs
*/
func main() {
	ctx := context.Background()

	acc, opts, err := gcputils.AccountAndCredentialsFromEnv("G_KEY")
	if err != nil {
		log.Fatal(err)
	}
	// lc, err := gcputils.InitLogging(ctx, projectID, opts)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer lc.Close()
	fireapp, err := firetils.New(ctx, acc.ProjectID, opts)
	if err != nil {
		log.Fatalf("couldn't init firebase app: %v\n", err)
	}
	firestore, err := fireapp.Firestore(ctx)
	if err != nil {
		log.Fatalf("couldn't init firestore: %v\n", err)
	}

	rpcClient, err := rpc.Dial(rpcURL)
	if err != nil {
		log.Fatalf("failed to dial rpc %q: %v", rpcURL, err)
	}
	rpc := goclient.NewClient(rpcClient)

	err = collector.FetchData(ctx, rpc, firestore)
	if err != nil {
		gotils.C(ctx).Printf("error on FetchData: %v", err)
	}

}
