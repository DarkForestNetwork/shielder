package contract

// This file adds some custom methods to the abigen generated ConfigContract class

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// BatchParams desribes the parameters to be used for a single batch
type BatchParams struct {
	BatchIndex  uint64
	StartBlock  uint64
	EndBlock    uint64
	BatchConfig *BatchConfig
}

// KeyperIndex returns the index of the keyper identified by the given address
func (bc *BatchConfig) KeyperIndex(address common.Address) (uint64, bool) {
	for i, k := range bc.Keypers {
		if k == address {
			return uint64(i), true
		}
	}
	return 0, false
}

func makeBatchParams(bc *BatchConfig, batchIndex uint64) (BatchParams, error) {
	batchSpan := bc.BatchSpan
	startBatchIndex := bc.StartBatchIndex
	if batchIndex < startBatchIndex {
		return BatchParams{}, fmt.Errorf("bad parameters: %d %d", batchIndex, startBatchIndex)
	}
	startBlock := bc.StartBlockNumber + batchSpan*(batchIndex-startBatchIndex)
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
	bc, err := cc.GetConfig(opts, batchIndex)
	if err != nil {
		return BatchParams{}, err
	}
	return makeBatchParams(&bc, batchIndex)
}

// NextBatchIndex determines the next batch index to be started after the given block number.
func (cc *ConfigContract) NextBatchIndex(blockNumber uint64) (uint64, error) {
	numConfigs, err := cc.NumConfigs(nil)
	if err != nil {
		return 0, err
	}
	for i := numConfigs - 1; i >= 0; i-- {
		cfg, err := cc.Configs(nil, big.NewInt(0).SetUint64(i))
		if err != nil {
			return 0, err
		}

		startBlockNumber := cfg.StartBlockNumber
		batchSpan := cfg.BatchSpan
		if batchSpan == 0 {
			return cfg.StartBatchIndex, nil
		}
		if startBlockNumber <= blockNumber {
			next := cfg.StartBatchIndex + (blockNumber-startBlockNumber+batchSpan-1)/batchSpan
			return next, nil
		}
	}
	return uint64(0), fmt.Errorf("contract misconfigured")
}

// GetConfigKeypers queries the list of keypers defined in the config given by its index
func (cc *ConfigContract) GetConfigKeypers(opts *bind.CallOpts, configIndex uint64) ([]common.Address, error) {
	var keypers []common.Address

	numKeypers, err := cc.ConfigNumKeypers(opts, configIndex)
	if err != nil {
		return keypers, err
	}

	for i := uint64(0); i < numKeypers; i++ {
		keyper, err := cc.ConfigKeypers(opts, configIndex, i)
		if err != nil {
			return keypers, err
		}
		keypers = append(keypers, keyper)
	}

	return keypers, nil
}

// GetConfigByIndex queries the batch config by its index (not the batch index, but the config index)
func (cc *ConfigContract) GetConfigByIndex(opts *bind.CallOpts, configIndex uint64) (BatchConfig, error) {
	config, err := cc.Configs(opts, big.NewInt(0).SetUint64(configIndex))
	if err != nil {
		return BatchConfig{}, err
	}

	keypers, err := cc.GetConfigKeypers(opts, configIndex)
	if err != nil {
		return BatchConfig{}, err
	}

	return BatchConfig{
		StartBatchIndex:        config.StartBatchIndex,
		StartBlockNumber:       config.StartBlockNumber,
		Threshold:              config.Threshold,
		BatchSpan:              config.BatchSpan,
		BatchSizeLimit:         config.BatchSizeLimit,
		TransactionSizeLimit:   config.TransactionSizeLimit,
		TransactionGasLimit:    config.TransactionGasLimit,
		FeeReceiver:            config.FeeReceiver,
		TargetAddress:          config.TargetAddress,
		TargetFunctionSelector: config.TargetFunctionSelector,
		ExecutionTimeout:       config.ExecutionTimeout,
		Keypers:                keypers,
	}, nil
}
