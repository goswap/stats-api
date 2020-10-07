package collector

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gochain/gochain/v3/common"
	"github.com/gochain/gochain/v3/goclient"
	"github.com/goswap/stats-api/backend"
	"github.com/goswap/stats-api/contracts"
	"github.com/goswap/stats-api/models"
	"github.com/goswap/stats-api/utils"
	"github.com/shopspring/decimal"
	"github.com/treeder/gotils"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	factoryAddress     = "0xe93c2cD333902d8dd65bF9420B68fC7B1be94bB3"
	maxBlockPerRequest = int64(10000)
)

var (
	mu        sync.RWMutex
	TokenMap  = map[string]*models.Token{}
	USDCPairs = map[string]*models.Pair{}
)

// GetPairsFromChain returns all the pairs that have been registered in GoSwap via the Factory contract
// fromIndex will get all the pairs from the current index. Pass in 0 for all.
func GetPairsFromChain(ctx context.Context, rpc *goclient.Client, fromIndex int) ([]*models.Pair, error) {
	addr := common.HexToAddress(factoryAddress)
	mainContract, err := contracts.NewUniswapFactory(addr, rpc)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on NewUniswapFactory: %v", err)
	}
	pairsLength, err := mainContract.AllPairsLength(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on AllPairsLength: %v", err)
	}
	fmt.Printf("Factory contract number of pairs: %v\n", pairsLength.String())

	pairs := []*models.Pair{}
	{ // doing this so the errgroup ctx doesn't affect the rest
		g, ctx := errgroup.WithContext(ctx)
		for i := fromIndex; i < int(pairsLength.Int64()); i++ {
			i2 := i // https://golang.org/doc/faq#closures_and_goroutines
			pairIndex := big.NewInt(int64(i))
			g.Go(func() error {
				address, err := mainContract.AllPairs(nil, pairIndex)
				if err != nil {
					return gotils.C(ctx).Errorf("error on AllPairs call: %v", err)
				}
				fmt.Printf("%v PAIR: %v\n", pairIndex, address.String())
				pair, err := GetPairDetails(ctx, rpc, address)
				if err != nil {
					return gotils.C(ctx).Errorf("failed to get token details: %v", err)
				}
				pair.Index = i2
				fmt.Printf("%v %v %v -- %v %v\n", pairIndex, pair.Token0.Symbol, pair.Token0.Name, pair.Token1.Symbol, pair.Token1.Name)
				fmt.Printf("%v Addresses: %v -- %v\n", pairIndex, pair.Token0.Address.Hex(), pair.Token1.Address.Hex())

				mu.Lock()
				defer mu.Unlock()
				pairs = append(pairs, pair)
				TokenMap[pair.Token0.Address.Hex()] = pair.Token0
				TokenMap[pair.Token1.Address.Hex()] = pair.Token1
				// save USDC pairs for valuations
				// DON'T REMOVE THIS FROM HERE YET, USED IN OTHER STUFF FOR NOW
				if pair.Token0.Symbol == "USDC" {
					USDCPairs[pair.Token1.Symbol] = pair
					p, err := pair.PriceInUSD(ctx)
					if err != nil {
						return gotils.C(ctx).Errorf("error getting price: %v", err)
					}
					fmt.Printf("Current price of %v: %v\n", pair.Token1.Symbol, p)
				}
				if pair.Token1.Symbol == "USDC" {
					USDCPairs[pair.Token0.Symbol] = pair
					p, err := pair.PriceInUSD(ctx)
					if err != nil {
						return gotils.C(ctx).Errorf("error getting price: %v", err)
					}
					fmt.Printf("Current price of %v: %v\n", pair.Token0.Symbol, p)
				}
				// END DON'T REMOVE
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
	}
	return pairs, nil
}

func PriceInUSD(ctx context.Context, symbol string) (decimal.Decimal, error) {
	if symbol == "USDC" {
		return decimal.NewFromInt(1), nil
	}
	p := USDCPairs[symbol]
	if p == nil {
		return decimal.Zero, &models.PriceNotFound{}
	}
	p2, err := p.PriceInUSD(ctx)
	// if err == nil {
	// 	priceMap[symbol] = p2
	// }
	return p2, err
}

func GetPairDetails(ctx context.Context, rpc *goclient.Client, contractAddress common.Address) (*models.Pair, error) {
	tb := &models.Pair{
		Address: contractAddress,
	}
	p, err := contracts.NewPair(contractAddress, rpc)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on NewPair: %v", err)
	}
	tb.PairContract = p
	t0address, err := p.Token0(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("Token0: %v", err)
	}
	t1address, err := p.Token1(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("Token1: %v", err)
	}
	mu.RLock()
	tb.Token0 = TokenMap[t0address.Hex()]
	mu.RUnlock()
	if tb.Token0 == nil {
		tb.Token0, err = GetErc20Details(ctx, rpc, t0address)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("get erc20 details: %v", err)
		}
	}
	mu.RLock()
	tb.Token1 = TokenMap[t1address.Hex()]
	mu.RUnlock()
	if tb.Token1 == nil {
		tb.Token1, err = GetErc20Details(ctx, rpc, t1address)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("get erc20 details: %v", err)
		}
	}
	tb.Pair = tb.String()
	return tb, err
}

func GetErc20Details(ctx context.Context, rpc *goclient.Client, addr common.Address) (*models.Token, error) {
	t0erc20, err := contracts.NewERC20(addr, rpc)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("Token0: %v", err)
	}
	td := &models.Token{Address: addr}
	td.Name, err = t0erc20.Name(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("%v", err) // need a Wrap or Err method if we don't want to add to the string
	}
	td.Symbol, err = t0erc20.Symbol(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("%v", err) // need a Wrap or Err method if we don't want to add to the string
	}
	td.Decimals, err = t0erc20.Decimals(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("%v", err) // need a Wrap or Err method if we don't want to add to the string
	}
	return td, nil
}

type LastCheck struct {
	LastCheckAt     time.Time `firestore:"lastCheckAt" json:"lastCheckAt"`
	LastBlockNumber int64     `firestore:"lastBlockNumber"  json:"lastBlockNumber"`
}

// FetchData is the main data collection function
func FetchData(ctx context.Context, rpc *goclient.Client, fs *firestore.Client) error {
	var startBlock int64

	// set this to how big we want the buckets
	truncateBy := time.Hour

	endBlock := int64(0)
	num, err := rpc.LatestBlockNumber(ctx)
	if err != nil {
		return gotils.C(ctx).Errorf("WARN: Failed to get latest block number: %v", err)
	}
	endBlock = num.Int64()
	latestBlockTimestamp, err := GetTimestampByBlockNumber(ctx, rpc, endBlock)
	if err != nil {
		return gotils.C(ctx).Errorf("%v", err)
	}
	stopAt := latestBlockTimestamp.Truncate(truncateBy) // don't process past this

	lcRef := fs.Collection(backend.CollectionTimestamps).Doc("last_check")
	dsnap, err := lcRef.Get(ctx)
	lc := &LastCheck{}
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return gotils.C(ctx).Errorf("error getting lastcheck: %v", err)
		}
		// no lastcheck, so let's start at a recent block
		num, err := rpc.LatestBlockNumber(ctx)
		if err != nil {
			return gotils.C(ctx).Errorf("Failed to get latest block number: %v", err)
		}
		startBlock = num.Int64() - (720 * 2) // about 2 hours, going back 2 because stopAt will prevent the final hour
		fmt.Printf("no last check\n")
	} else {
		err = dsnap.DataTo(lc)
		if err != nil {
			return gotils.C(ctx).Errorf("Failed to DataTo: %v", err)
		}
		if lc.LastCheckAt.Equal(stopAt) {
			// too soon...
			fmt.Printf("Last check == stopAt, cancelling...\n")
			return nil
		}
		startBlock = lc.LastBlockNumber + 1
		fmt.Printf("Last check at %v\nStarting at block %v\n", lc.LastCheckAt, startBlock)
	}
	if startBlock == 0 {
		return gotils.C(ctx).Errorf("startBlock is zero")
	}

	fmt.Printf("fetching from block %v to %v\n", startBlock, endBlock)

	db, _ := backend.NewFirestore(ctx, fs)

	// load tokens into the cache
	_, err = db.GetTokens(ctx)
	if err != nil {
		return gotils.C(ctx).Errorf("error on GetTokens: %v", err)
	}

	pairs, err := db.GetPairs(ctx)
	if err != nil {
		return gotils.C(ctx).Errorf("error on GetPairs: %v", err)
	}
	// get any new pairs
	newPairs, err := GetPairsFromChain(ctx, rpc, len(pairs)) // will return any new pairs
	if err != nil {
		return gotils.C(ctx).Errorf("error on GetAllPairs: %v", err)
	}
	// store any new pairs
	err = storePairs(ctx, rpc, fs, newPairs)
	if err != nil {
		return gotils.C(ctx).Errorf("error on storePairs: %v", err)
	}
	pairs = append(pairs, newPairs...)

	pairMap := map[string]*models.Pair{}
	// set USDCPairs for prices
	for _, pair := range pairs {
		pc, err := contracts.NewPair(pair.Address, rpc)
		if err != nil {
			return gotils.C(ctx).Errorf("error on contracts.NewPair: %v", err)
		}
		pair.PairContract = pc
		if pair.Token0.Symbol == "USDC" {
			USDCPairs[pair.Token1.Symbol] = pair
			p, err := pair.PriceInUSD(ctx)
			if err != nil {
				return gotils.C(ctx).Errorf("error getting price: %v", err)
			}
			fmt.Printf("Current price of %v: %v\n", pair.Token1.Symbol, p)
		}
		if pair.Token1.Symbol == "USDC" {
			USDCPairs[pair.Token0.Symbol] = pair
			p, err := pair.PriceInUSD(ctx)
			if err != nil {
				return gotils.C(ctx).Errorf("error getting price: %v", err)
			}
			fmt.Printf("Current price of %v: %v\n", pair.Token0.Symbol, p)
		}
		pairMap[pair.Address.Hex()] = pair
	}

	mostRecentBlockProcessed := startBlock
	// tokenMap := map[string]*Token{}

	totalBuckets := map[int64]*models.TotalBucket{}
	pairBucketsMap := map[common.Address]map[int64]*models.PairBucket{}
	tokenBucketsMap := map[common.Address]map[int64]*models.TokenBucket{}
	pairLiquidities := map[common.Address]*models.PairLiquidity{}
	totalLiquidityUSD := decimal.Zero
	for _, p := range pairs {

		pairLiquidity, err := fetchLiquidity(ctx, rpc, fs, p)
		if err != nil {
			return gotils.C(ctx).Errorf("error on fetchLiquidity: %v", err)
		}
		pairLiquidities[p.Address] = pairLiquidity
		fmt.Printf("%v liquidity: %v\n", p.String(), pairLiquidity.ValUSD())
		totalLiquidityUSD = totalLiquidityUSD.Add(pairLiquidity.ValUSD())

		swapEvents, err := GetSwapEvents(ctx, rpc, p.Address, startBlock, endBlock)
		if err != nil {
			return gotils.C(ctx).Errorf("error on GetSwapEvents: %v", err)
		}
		fmt.Printf("%v swap events for %v\n", len(swapEvents), p.String())

		pairBuckets := pairBucketsMap[p.Address]
		if pairBuckets == nil {
			pairBuckets = map[int64]*models.PairBucket{}
			pairBucketsMap[p.Address] = pairBuckets
		}
		bucketsMade := 0
		for _, ev := range swapEvents {
			// Stop processing the last bucket, since it'll most likely be partial
			if !ev.Timestamp.Before(stopAt) { // using before so it doesn't include if it's equal
				fmt.Printf("stopping since event is after stopAt")
				break
			}
			// tally up for each pair
			// tally up for each token
			// tally up totals across all pools
			bucketTime := ev.Timestamp.Truncate(truncateBy)
			// fmt.Printf("bucketTime: %v\n", bucketTime)

			ut := bucketTime.Unix()
			pairBucket := pairBuckets[ut]
			if pairBucket == nil {
				pairBucket = &models.PairBucket{Address: p.Address.Hex(), Pair: p.String(), Time: bucketTime}
				pairBuckets[ut] = pairBucket
				pairBucket.Price0USD, err = PriceInUSD(ctx, p.Token0.Symbol)
				if err != nil {
					gotils.C(ctx).Printf("error getting price for %v: %v\n", p.Token0.Symbol, err)
				}
				pairBucket.Price1USD, err = PriceInUSD(ctx, p.Token1.Symbol)
				if err != nil {
					gotils.C(ctx).Printf("error getting price for %v: %v\n", p.Token1.Symbol, err)
				}

				// liquidity
				pairBucket.Reserve0 = pairLiquidity.Reserve0
				pairBucket.Reserve1 = pairLiquidity.Reserve1
				pairBucket.TotalSupply = pairLiquidity.TotalSupply

			}
			amount0In := utils.IntToDec(ev.Amount0In, p.Token0.Decimals)
			amount1In := utils.IntToDec(ev.Amount1In, p.Token1.Decimals)
			amount0Out := utils.IntToDec(ev.Amount0Out, p.Token0.Decimals)
			amount1Out := utils.IntToDec(ev.Amount1Out, p.Token1.Decimals)
			pairBucket.Amount0In = pairBucket.Amount0In.Add(amount0In)
			pairBucket.Amount1In = pairBucket.Amount1In.Add(amount1In)
			pairBucket.Amount0Out = pairBucket.Amount0Out.Add(amount0Out)
			pairBucket.Amount1Out = pairBucket.Amount1Out.Add(amount1Out)

			volumeUSD := amount0In.Mul(pairBucket.Price0USD).Add(amount1In.Mul(pairBucket.Price1USD))
			pairBucket.VolumeUSD = pairBucket.VolumeUSD.Add(volumeUSD)
			bucketsMade++

			if ev.BlockNumber > mostRecentBlockProcessed {
				mostRecentBlockProcessed = ev.BlockNumber
			}
		}
		fmt.Printf("%v PairBuckets made: %v\n", p.String(), bucketsMade)
		if bucketsMade == 0 {
			// we'll make one for prior hour, just so we get the liquidity right
			fmt.Printf("Making PairBucket for liquidity\n")

			bucketTime := time.Now().Add(-(1 * time.Hour)).Truncate(truncateBy)
			ut := bucketTime.Unix()
			pairBucket := &models.PairBucket{Address: p.Address.Hex(), Pair: p.String(), Time: bucketTime}
			pairBuckets[ut] = pairBucket
			pairBucket.Price0USD, err = PriceInUSD(ctx, p.Token0.Symbol)
			if err != nil {
				gotils.C(ctx).Printf("error getting price for %v: %v\n", p.Token0.Symbol, err)
			}
			pairBucket.Price1USD, err = PriceInUSD(ctx, p.Token1.Symbol)
			if err != nil {
				gotils.C(ctx).Printf("error getting price for %v: %v\n", p.Token1.Symbol, err)
			}

			// liquidity
			pairBucket.Reserve0 = pairLiquidity.Reserve0
			pairBucket.Reserve1 = pairLiquidity.Reserve1
			pairBucket.TotalSupply = pairLiquidity.TotalSupply
		}

		// fmt.Printf("buckets for %v\n\n", p.String())
		tokenBuckets0 := tokenBucketsMap[p.Token0.Address]
		if tokenBuckets0 == nil {
			tokenBuckets0 = map[int64]*models.TokenBucket{}
			tokenBucketsMap[p.Token0.Address] = tokenBuckets0
		}
		tokenBuckets1 := tokenBucketsMap[p.Token1.Address]
		if tokenBuckets1 == nil {
			tokenBuckets1 = map[int64]*models.TokenBucket{}
			tokenBucketsMap[p.Token1.Address] = tokenBuckets1
		}

		for t, v := range pairBuckets {
			// fmt.Printf("%v %v 0in: %v 1in: %v, 0out: %v 1out: %v\n", t, v.Time, v.Amount0In, v.Amount1In, v.Amount0Out, v.Amount1Out)
			t2 := time.Unix(t, 0)
			// fmt.Printf("t2: %v\n", t2)
			// token0
			{
				tokenBucket0 := tokenBuckets0[t]
				if tokenBucket0 == nil {
					tokenBucket0 = &models.TokenBucket{Address: p.Token0.Address.Hex(), Symbol: p.Token0.Symbol, Time: t2}
					tokenBucket0.PriceUSD = v.Price0USD
					tokenBuckets0[t] = tokenBucket0
				}
				tokenBucket0.AmountIn = tokenBucket0.AmountIn.Add(v.Amount0In)
				tokenBucket0.AmountOut = tokenBucket0.AmountOut.Add(v.Amount0Out)
				volumeUSD := v.Amount0In.Mul(v.Price0USD)
				tokenBucket0.VolumeUSD = tokenBucket0.VolumeUSD.Add(volumeUSD)
			}

			// token1
			{
				tokenBucket1 := tokenBuckets1[t]
				if tokenBucket1 == nil {
					tokenBucket1 = &models.TokenBucket{Address: p.Token1.Address.Hex(), Symbol: p.Token1.Symbol, Time: t2}
					tokenBucket1.PriceUSD = v.Price1USD
					tokenBuckets1[t] = tokenBucket1
				}
				tokenBucket1.AmountIn = tokenBucket1.AmountIn.Add(v.Amount1In)
				tokenBucket1.AmountOut = tokenBucket1.AmountOut.Add(v.Amount1Out)
				volumeUSD := v.Amount1In.Mul(v.Price1USD)
				tokenBucket1.VolumeUSD = tokenBucket1.VolumeUSD.Add(volumeUSD)
			}

			// totals
			totalBucket := totalBuckets[t]
			if totalBucket == nil {
				totalBucket = &models.TotalBucket{Time: t2}
				totalBuckets[t] = totalBucket
			}
			totalBucket.VolumeUSD = totalBucket.VolumeUSD.Add(v.VolumeUSD)
		}
		for _, tb := range tokenBuckets0 {
			tb.Reserve = tb.Reserve.Add(pairLiquidity.Reserve0)
		}
		for _, tb := range tokenBuckets1 {
			tb.Reserve = tb.Reserve.Add(pairLiquidity.Reserve1)
		}

	}

	// TODO: store all data in db here
	fmt.Printf("\nSTORE PAIR DATA:\n\n")
	v := decimal.Zero
	for pairAddress, pbs := range pairBucketsMap {
		fmt.Printf("Pair: %s\n", pairAddress.Hex())
		vol := decimal.Zero
		for t, pb := range pbs {
			t2 := time.Unix(t, 0)
			fmt.Printf("time bucket: %v -- %v\n", t, t2)
			pb.PreSave()
			_, err = fs.Collection(backend.CollectionPairBuckets).Doc(fmt.Sprintf("%v_%v", pairAddress.Hex(), t)).Set(ctx, pb)
			if err != nil {
				return gotils.C(ctx).Errorf("error writing to db: %v", err)
			}
			vol = vol.Add(pb.VolumeUSD)
		}
		v = v.Add(vol)
		fmt.Printf("Volume: %v\n", vol.StringFixed(2))
	}
	fmt.Printf("total: %v\n", v)

	fmt.Printf("\nSTORE TOKEN DATA:\n\n")
	v = decimal.Zero
	for address, pbs := range tokenBucketsMap {
		fmt.Printf("Token: %v\n", address.Hex())
		vol := decimal.Zero
		for t, pb := range pbs {
			vol = vol.Add(pb.VolumeUSD)
			t2 := time.Unix(t, 0)
			fmt.Printf("time bucket: %v -- %v\n", t, t2)
			pb.PreSave()
			_, err = fs.Collection(backend.CollectionTokenBuckets).Doc(fmt.Sprintf("%v_%v", address.Hex(), t)).Set(ctx, pb)
			if err != nil {
				return gotils.C(ctx).Errorf("error writing to db: %v", err)
			}
		}
		v = v.Add(vol)
		fmt.Printf("Volume: %v\n", vol.StringFixed(2))
	}
	fmt.Printf("total: %v\n", v)

	{
		fmt.Printf("\nTOTALS:\n\n")
		vol := decimal.Zero
		for t, pb := range totalBuckets {
			pb.LiquidityUSD = totalLiquidityUSD
			pb.PreSave()
			_, err = fs.Collection(backend.CollectionTotals).Doc(fmt.Sprintf("%v", t)).Set(ctx, pb)
			if err != nil {
				return gotils.C(ctx).Errorf("error writing to db: %v", err)
			}
			vol = vol.Add(pb.VolumeUSD)
		}
		fmt.Printf("Volume: %v liquidity: %v\n", vol.StringFixed(2), totalLiquidityUSD.StringFixed(2))
	}

	// TODO: save last_check
	lc = &LastCheck{
		LastCheckAt:     stopAt, // setting this so it will
		LastBlockNumber: mostRecentBlockProcessed,
	}
	lcRef.Set(ctx, lc)
	return nil

}

func fetchLiquidity(ctx context.Context, rpc *goclient.Client, fs *firestore.Client, pair *models.Pair) (*models.PairLiquidity, error) {
	t0 := pair.Token0
	price0, err := PriceInUSD(ctx, t0.Symbol)
	if err != nil {
		var e *models.PriceNotFound
		if errors.As(err, &e) {
			// err is a *QueryError, and e is set to the error's value
			fmt.Printf("%v price error: %v\n", t0.Symbol, e)
		} else {
			return nil, gotils.C(ctx).Errorf("error getting price0 for %v: %v", t0.Symbol, err)
		}
	}
	t1 := pair.Token1
	price1, err := PriceInUSD(ctx, t1.Symbol)
	if err != nil {
		var e *models.PriceNotFound
		if errors.As(err, &e) {
			// err is a *QueryError, and e is set to the error's value
			fmt.Printf("%v price error: %v\n", t0.Symbol, e)
		} else {
			return nil, gotils.C(ctx).Errorf("error getting price1 for %v: %v", t1.Symbol, err)
		}
	}
	reserve0, reserve1, err := pair.GetReserves(ctx)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on GetReserves: %v", err)
	}
	totalSupplyBig, err := pair.PairContract.TotalSupply(nil)
	if err != nil {
		return nil, gotils.C(ctx).Errorf("error on TotalSupply: %v", err)
	}
	totalSupply := utils.IntToDec(totalSupplyBig, 18)

	poolVal := &models.PairLiquidity{
		Address: pair.Address.Hex(),
		Pair:    pair.String(),
		// Token0:      pair.Token0.Symbol,
		// Token1:      pair.Token1.Symbol,
		TotalSupply: totalSupply,
		Reserve0:    reserve0,
		Reserve1:    reserve1,
		Price0USD:   price0,
		Price1USD:   price1,
	}
	return poolVal, nil
}

func storePairs(ctx context.Context, rpc *goclient.Client, fs *firestore.Client, pairs []*models.Pair) error {
	for _, p := range pairs {
		fmt.Printf("Storing new pair: %v, index: %v\n", p.String(), p.Index)
		// store tokens too while we're at it
		t := p.Token0
		t.PreSave()
		_, err := fs.Collection(backend.CollectionTokens).Doc(t.Address.Hex()).Set(ctx, t)
		if err != nil {
			return gotils.C(ctx).Errorf("error storing pair: %v", err)
		}
		t = p.Token1
		t.PreSave()
		_, err = fs.Collection(backend.CollectionTokens).Doc(t.Address.Hex()).Set(ctx, t)
		if err != nil {
			return gotils.C(ctx).Errorf("error storing pair: %v", err)
		}
		p.PreSave()
		_, err = fs.Collection(backend.CollectionPairs).Doc(p.Address.Hex()).Set(ctx, p)
		if err != nil {
			return gotils.C(ctx).Errorf("error storing pair: %v", err)
		}
	}
	return nil
}
