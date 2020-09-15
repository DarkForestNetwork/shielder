package contract

// This file adds some custom methods to the abigen generated ConfigContract class

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// BatchParams desribes the parameters to be used for a single batch
type BatchParams struct {
	BatchIndex  uint64
	StartBlock  uint64
	EndBlock    uint64
	BatchConfig *BatchConfig
}

func makeBatchParams(bc *BatchConfig, batchIndex uint64) (BatchParams, error) {
	batchSpan := bc.BatchSpan.Uint64()
	startBatchIndex := bc.StartBatchIndex.Uint64()
	if batchIndex < startBatchIndex {
		return BatchParams{}, fmt.Errorf("bad parameters: %d %d", batchIndex, startBatchIndex)
	}
	startBlock := bc.StartBlockNumber.Uint64() + batchSpan*(batchIndex-startBatchIndex)
	endBlock := startBlock + batchSpan
	return BatchParams{
		BatchIndex:  batchIndex,
		StartBlock:  startBlock,
		EndBlock:    endBlock,
		BatchConfig: bc,
	}, nil
}

// QueryBatchParams queries the config contract for the batch parameters of the given batch index.
func (cc *ConfigContract) QueryBatchParams(opts *bind.CallOpts, batchIndex uint64) (BatchParams, error) {
	bc, err := cc.GetConfig(opts, big.NewInt(0).SetUint64(batchIndex))
	if err != nil {
		return BatchParams{}, err
	}
	return makeBatchParams(&bc, batchIndex)
}

// NextBatchIndex determines the next batch index to be started after the given block number.
func (cc *ConfigContract) NextBatchIndex(blockNumber uint64) (uint64, error) {
	_numConfigs, err := cc.NumConfigs(nil)
	if err != nil {
		return 0, err
	}
	numConfigs := _numConfigs.Int64()
	for i := numConfigs - 1; i >= 0; i-- {
		cfg, err := cc.Configs(nil, big.NewInt(i))
		if err != nil {
			return 0, err
		}

		startBlockNumber := cfg.StartBlockNumber.Uint64()
		batchSpan := cfg.BatchSpan.Uint64()
		if batchSpan == 0 {
			return 0, fmt.Errorf("no active config for block %d", blockNumber)
		}
		if startBlockNumber <= blockNumber {
			next := cfg.StartBatchIndex.Uint64() + (blockNumber-startBlockNumber+batchSpan-1)/batchSpan
			return next, nil
		}
	}
	return uint64(0), fmt.Errorf("contract misconfigured")
}
