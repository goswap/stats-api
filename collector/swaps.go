package collector

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/gochain-io/explorer/server/utils"
	"github.com/gochain/gochain/v3"
	"github.com/gochain/gochain/v3/accounts/abi"
	"github.com/gochain/gochain/v3/common"
	"github.com/gochain/gochain/v3/core/types"
	"github.com/gochain/gochain/v3/goclient"
	"github.com/goswap/stats-api/contracts"
	"github.com/treeder/gotils/v2"
)

// SwapEvent represents an emitted Swap event
type SwapEvent struct {
	TxFrom          common.Address // this user who initiated this transaction, aka: origin
	From            common.Address // this will typically be another contract, ie: the router
	To              common.Address
	BlockNumber     int64
	TransactionHash string
	Amount0In       *big.Int
	Amount1In       *big.Int
	Amount0Out      *big.Int
	Amount1Out      *big.Int
	Timestamp       time.Time
}

var swapEventID = common.HexToHash("0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822")

func GetSwapEvents(ctx context.Context, rpc *goclient.Client, pairAddress common.Address, startBlock, endBlock int64) ([]*SwapEvent, error) {
	abi, err := abi.JSON(strings.NewReader(contracts.PairABI))
	if err != nil {
		fmt.Println("Failed to parse token Uniswap ABI:", err)
		os.Exit(1)
	}
	ctx = gotils.With(ctx, "address", pairAddress)
	var swapEvents []*SwapEvent
	numOfBlocksPerRequest := maxBlockPerRequest

	currentBlock := startBlock
	for currentBlock <= endBlock {
		toBlock := currentBlock + numOfBlocksPerRequest
		if toBlock > endBlock {
			toBlock = endBlock
		}
		fmt.Printf("Querying for swap events, from: %v, to: %v\n",
			currentBlock, toBlock)
		query := gochain.FilterQuery{
			FromBlock: big.NewInt(currentBlock),
			ToBlock:   big.NewInt(toBlock),
			Addresses: []common.Address{pairAddress},
			Topics:    [][]common.Hash{{swapEventID}},
		}

		var logs []types.Log
		err := utils.Retry(ctx, 5, 2*time.Second, func() (err error) {
			logs, err = rpc.FilterLogs(ctx, query)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query RPC for logs: %v", err)
		}
		var currentTimeStamp time.Time
		var currentBlockNumber int64
		for _, log := range logs {
			event, err := unpackSwapEvent(ctx, abi, log)
			if err != nil {
				return nil, gotils.C(ctx).Errorf("Failed to unpack event: %v, block: %v, index: %v, contract: %v", err, log.BlockNumber, log.Index, log.Address)
			}
			swapEvents = append(swapEvents, event)

			// get who the tx was from
			tx, _, err := rpc.TransactionByHashFull(ctx, log.TxHash)
			if err != nil {
				return nil, gotils.C(ctx).Errorf("Failed to get transaction: %v", err)
			}
			event.TxFrom = *tx.From

			// todo: get timestamp if block number has changed, see below, set it on all events until it changes again
			if currentBlockNumber != event.BlockNumber || currentTimeStamp.IsZero() {
				// then get new timestamp
				currentTimeStamp, err = GetTimestampByBlockNumber(ctx, rpc, event.BlockNumber)
			}
			event.Timestamp = currentTimeStamp
			currentBlockNumber = event.BlockNumber
		}
		currentBlock = toBlock + 1
	}
	return swapEvents, nil
}

var addrTopicPrefix = make([]byte, 12)

func unpackSwapEvent(ctx context.Context, abi abi.ABI, event types.Log) (*SwapEvent, error) {
	if l := len(event.Topics); l != 3 {
		return nil, fmt.Errorf("incorrect number of topics: %d", l)
	}
	from := event.Topics[1].Bytes()
	to := event.Topics[2].Bytes()
	if len(bytes.TrimPrefix(from, addrTopicPrefix)) != 20 {
		return nil, fmt.Errorf("from topic longer than address: %s", string(from))
	}
	if len(bytes.TrimPrefix(to, addrTopicPrefix)) != 20 {
		return nil, fmt.Errorf("to topic longer than address: %s", string(from))
	}
	var swapEvent SwapEvent
	err := abi.UnpackIntoInterface(&swapEvent, "Swap", event.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack log data: %v", err)
	}
	swapEvent.From = common.BytesToAddress(from)
	swapEvent.To = common.BytesToAddress(to)
	swapEvent.BlockNumber = int64(event.BlockNumber)
	swapEvent.TransactionHash = event.TxHash.String()
	return &swapEvent, nil
}

func GetTimestampByBlockNumber(ctx context.Context, rpc *goclient.Client, blockNumber int64) (time.Time, error) {
	bln := new(big.Int).SetInt64(blockNumber)
	block, err := rpc.BlockByNumber(ctx, bln)
	if err != nil {
		return time.Time{}, err
	}
	t := time.Unix(block.Time().Int64(), 0)
	return t, nil
}
