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

// MintEvent represents a Mint event emitted from an Uniswap.
type MintEvent struct {
	TxFrom          common.Address // actual user who sent it
	Sender          common.Address
	FromContract    common.Address
	BlockNumber     int64
	TransactionHash string
	TxHash          common.Hash
	Amount0         *big.Int
	Amount1         *big.Int
}

var mintEventID = common.HexToHash("0x4c209b5fc8ad50758f13e2e1088ba56a560dff690a1c6fef26394f4c03821c4f")

func GetMintEvents(ctx context.Context, rpc *goclient.Client, pairAddress common.Address, startBlock, endBlock int64, blockLimit uint64) ([]*MintEvent, error) {
	abi, err := abi.JSON(strings.NewReader(contracts.PairABI))
	if err != nil {
		fmt.Println("Failed to parse token Uniswap ABI:", err)
		os.Exit(1)
	}
	ctx = gotils.With(ctx, "address", pairAddress)
	numOfBlocksPerRequest := int64(blockLimit)
	if endBlock <= 0 {
		num, err := rpc.LatestBlockNumber(ctx)
		if err != nil {
			return nil, gotils.C(ctx).Errorf("WARN: Failed to get latest block number: %v", err)
		}
		endBlock = num.Int64()
	}
	var mintEvents []*MintEvent
	currentBlock := startBlock
	fmt.Printf("Start block: %v end block: %v\n", currentBlock, endBlock)
	for currentBlock <= endBlock {
		toBlock := currentBlock + numOfBlocksPerRequest
		if toBlock > endBlock {
			toBlock = endBlock
		}
		fmt.Printf("Querying for mint events, from: %v, to: %v\n",
			currentBlock, toBlock)
		// could try Pair.PairFilterer(..).MintFilter() then iterate over that.
		query := gochain.FilterQuery{
			FromBlock: big.NewInt(currentBlock),
			ToBlock:   big.NewInt(toBlock),
			Addresses: []common.Address{pairAddress},
			Topics:    [][]common.Hash{{mintEventID}},
		}

		var logs []types.Log
		err := utils.Retry(ctx, 5, 2*time.Second, func() (err error) {
			logs, err = rpc.FilterLogs(ctx, query)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query RPC for logs: %v", err)
		}
		for _, log := range logs {
			event, err := unpackMintEvent(ctx, abi, log)
			if err != nil {
				return nil, gotils.C(ctx).Errorf("Failed to unpack event: %v, block: %v, index: %v, contract: %v", err, log.BlockNumber, log.Index, log.Address)
			}
			mintEvents = append(mintEvents, event)

			// get who sent it for the mint to address
			tx, _, err := rpc.TransactionByHashFull(ctx, log.TxHash)
			if err != nil {
				return nil, gotils.C(ctx).Errorf("Failed to get transaction: %v", err)
			}
			event.TxFrom = *tx.From
			// fmt.Printf("tx: %v to: %v sender: %v from: %v\n", event.TransactionHash, event.TxFrom.Hex(), event.Sender.Hex(), event.FromContract.Hex())

		}
		currentBlock = toBlock + 1
	}
	return mintEvents, nil
}
func unpackMintEvent(ctx context.Context, abi abi.ABI, event types.Log) (*MintEvent, error) {
	if l := len(event.Topics); l != 2 {
		return nil, fmt.Errorf("incorrect number of topics: %d", l)
	}
	from := event.Topics[1].Bytes()

	if len(bytes.TrimPrefix(from, addrTopicPrefix)) != 20 {
		return nil, fmt.Errorf("from topic longer than address: %s", string(from))
	}
	var mintEvent MintEvent
	err := abi.UnpackIntoInterface(&mintEvent, "Mint", event.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack log data: %v", err)
	}
	mintEvent.FromContract = common.BytesToAddress(from)
	mintEvent.BlockNumber = int64(event.BlockNumber)
	mintEvent.TransactionHash = event.TxHash.String()
	mintEvent.TxHash = event.TxHash
	return &mintEvent, nil
}
