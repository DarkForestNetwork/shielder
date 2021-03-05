package fx

import (
	"context"

	"github.com/ethereum/go-ethereum/core/types"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/shmsg"
)

type RunEnv struct {
	MessageSender       MessageSender
	ContractCaller      *contract.Caller
	WatchedTransactions chan *types.Transaction
}

func (e RunEnv) SendMessage(ctx context.Context, msg *shmsg.Message) error {
	return e.MessageSender.SendMessage(ctx, msg)
}

func (e RunEnv) GetContractCaller(ctx context.Context) *contract.Caller {
	return e.ContractCaller
}

func (e RunEnv) WatchTransaction(tx *types.Transaction) {
	e.WatchedTransactions <- tx
}
