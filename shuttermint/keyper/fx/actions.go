package fx

import (
	"encoding/gob"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/observe"
	"shielder/shuttermint/shcrypto"
	"shielder/shuttermint/shmsg"
)

const (
	eonKeyBroadcastGasLimit     = uint64(1_000_000)
	executeCipherBatchBaseLimit = uint64(150_000)
	executePlainBatchBaseLimit  = uint64(150_000) // XXX check if we can lower that value
	skipCipherExecutionLimit    = uint64(100_000)
)

// IAction describes an action to run as determined by the Decider's Decide method.
type IAction interface {
	// IsExpired checks if the action expired because some time limit is reached or because the
	// result of the action has been achieved (we or someone else may have performed the same
	// action). We could think about having a dedicated IsDone method.
	IsExpired(world observe.World) bool
}

// MainChainTX is an action that sends a transaction to the main chain.
type MainChainTX interface {
	IAction
	SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error)
}

var (
	_ IAction     = SendShielderMessage{}
	_ MainChainTX = ExecuteCipherBatch{}
	_ MainChainTX = ExecutePlainBatch{}
	_ MainChainTX = SkipCipherBatch{}
	_ MainChainTX = Accuse{}
	_ MainChainTX = Appeal{}
	_ MainChainTX = EonKeyBroadcast{}
)

func init() {
	for _, a := range []IAction{
		&SendShielderMessage{},
		&ExecuteCipherBatch{},
		&ExecutePlainBatch{},
		&SkipCipherBatch{},
		&Accuse{},
		&Appeal{},
		&EonKeyBroadcast{},
	} {
		gob.Register(a)
	}
}

// SendShielderMessage is an Action that sends a message to shuttermint.
type SendShielderMessage struct {
	Description string
	Msg         *shmsg.Message
}

func (a SendShielderMessage) String() string {
	return fmt.Sprintf("=> shuttermint: %s", a.Description)
}

func (a SendShielderMessage) IsExpired(world observe.World) bool {
	return false
}

// ExecuteCipherBatch is an Action that instructs the executor contract to execute a cipher batch.
type ExecuteCipherBatch struct {
	BatchIndex          uint64
	CipherBatchHash     [32]byte
	Transactions        [][]byte
	KeyperIndex         uint64
	TransactionGasLimit uint64
}

func (a ExecuteCipherBatch) gasLimit() uint64 {
	return executeCipherBatchBaseLimit + uint64(len(a.Transactions))*a.TransactionGasLimit
}

func (a ExecuteCipherBatch) SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error) {
	auth.GasLimit = a.gasLimit()
	return caller.ExecutorContract.ExecuteCipherBatch(
		auth, a.BatchIndex, a.CipherBatchHash, a.Transactions, a.KeyperIndex,
	)
}

func (a ExecuteCipherBatch) String() string {
	return fmt.Sprintf("=> executor contract: execute cipher batch %d with %d txs", a.BatchIndex, len(a.Transactions))
}

func (a ExecuteCipherBatch) IsExpired(world observe.World) bool {
	halfStep := 2 * a.BatchIndex
	return world.MainChain.NumExecutionHalfSteps > halfStep
}

// ExecutePlainBatch is an Action that instructs the executor contract to execute a plain batch.
type ExecutePlainBatch struct {
	BatchIndex          uint64
	Transactions        [][]byte
	TransactionGasLimit uint64
}

func (a ExecutePlainBatch) gasLimit() uint64 {
	return executePlainBatchBaseLimit + uint64(len(a.Transactions))*a.TransactionGasLimit
}

func (a ExecutePlainBatch) SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error) {
	auth.GasLimit = a.gasLimit()
	return caller.ExecutorContract.ExecutePlainBatch(auth, a.BatchIndex, a.Transactions)
}

func (a ExecutePlainBatch) String() string {
	return fmt.Sprintf("=> executor contract: execute plain batch %d with %d txs", a.BatchIndex, len(a.Transactions))
}

func (a ExecutePlainBatch) IsExpired(world observe.World) bool {
	halfStep := 2*a.BatchIndex + 1
	return world.MainChain.NumExecutionHalfSteps > halfStep
}

// SkipCipherBatch is an Action that instructs the executor contract to skip a cipher batch.
type SkipCipherBatch struct {
	BatchIndex uint64
}

func (a SkipCipherBatch) SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error) {
	auth.GasLimit = skipCipherExecutionLimit
	return caller.ExecutorContract.SkipCipherExecution(auth, a.BatchIndex)
}

func (a SkipCipherBatch) String() string {
	return fmt.Sprintf("=> executor contract: skip cipher batch %d", a.BatchIndex)
}

func (a SkipCipherBatch) IsExpired(world observe.World) bool {
	halfStep := 2 * a.BatchIndex
	return world.MainChain.NumExecutionHalfSteps > halfStep
}

// Accuse is an action accusing the executor of a given half step at the keyper slasher.
type Accuse struct {
	HalfStep    uint64
	KeyperIndex uint64 // index of the accuser, not the executor
}

func (a Accuse) SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error) {
	return caller.KeyperSlasher.Accuse(auth, a.HalfStep, a.KeyperIndex)
}

func (a Accuse) String() string {
	return fmt.Sprintf("=> keyper slasher: accuse for half step %d", a.HalfStep)
}

func (a Accuse) IsExpired(world observe.World) bool {
	return false
}

// Appeal is an action countering an earlier invalid accusation.
type Appeal struct {
	Authorization contract.Authorization
}

func (a Appeal) SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error) {
	return caller.KeyperSlasher.Appeal(auth, a.Authorization)
}

func (a Appeal) String() string {
	return fmt.Sprintf("=> keyper slasher: appeal for half step %d", a.Authorization.HalfStep)
}

func (a Appeal) IsExpired(world observe.World) bool {
	return false
}

// EonKeyBroadcast is an action sending a vote for an eon public key to the key broadcast contract.
type EonKeyBroadcast struct {
	KeyperIndex     uint64
	StartBatchIndex uint64
	EonPublicKey    *shcrypto.EonPublicKey
}

func (a EonKeyBroadcast) SendTX(caller *contract.Caller, auth *bind.TransactOpts) (*types.Transaction, error) {
	authCopy := *auth
	authCopy.GasLimit = eonKeyBroadcastGasLimit
	return caller.KeyBroadcastContract.Vote(
		&authCopy,
		a.KeyperIndex,
		a.StartBatchIndex,
		a.EonPublicKey.Marshal(),
	)
}

func (a EonKeyBroadcast) String() string {
	return fmt.Sprintf("=> key broadcast contract: voting for eon key with start batch %d", a.StartBatchIndex)
}

func (a EonKeyBroadcast) IsExpired(world observe.World) bool {
	return false
}
