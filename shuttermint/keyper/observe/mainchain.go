package observe

import (
	"context"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/medley"
	"shielder/shuttermint/shcrypto"
)

// MainChain let's a keyper fetch all necessary information from an ethereum node to do it's
// work. The only source for the data stored in this struct should be the ethereum node.  The
// SyncToHead method can be used to update the data. All other accesses should be read-only.
type MainChain struct {
	FollowDistance          uint64
	CurrentBlock            uint64
	BatchConfigs            []contract.BatchConfig
	Batches                 map[uint64]*Batch
	NumExecutionHalfSteps   uint64
	CipherExecutionReceipts map[uint64]*contract.CipherExecutionReceipt
	Deposits                map[common.Address]*Deposit
	Accusations             map[uint64]*Accusation
}

// Batch stores the encrypted and plain transactions submitted to the batching contract for a
// particular index.
type Batch struct {
	BatchIndex            uint64
	EncryptedBatchHash    common.Hash
	EncryptedTransactions [][]byte
	PlainTransactions     [][]byte
	PlainBatchHash        common.Hash
}

// Deposit represents a deposit in the deposit contract.
type Deposit struct {
	Account                  common.Address
	Slashed                  bool
	Amount                   *big.Int
	WithdrawalDelayBlocks    uint64
	WithdrawalRequestedBlock uint64
}

// Accusation represents an accusation against a keyper in the keyper slasher.
type Accusation struct {
	Executor    common.Address
	Accuser     common.Address
	Appealed    bool
	HalfStep    uint64
	BlockNumber uint64
}

// NewMainChain creates an empty MainChain struct
func NewMainChain(followDistance uint64) *MainChain {
	return &MainChain{
		FollowDistance: followDistance,

		Batches:                 make(map[uint64]*Batch),
		CipherExecutionReceipts: make(map[uint64]*contract.CipherExecutionReceipt),
		Deposits:                make(map[common.Address]*Deposit),
		Accusations:             make(map[uint64]*Accusation),
	}
}

func (mainchain *MainChain) Clone() *MainChain {
	clone := new(MainChain)
	medley.CloneWithGob(mainchain, clone)
	return clone
}

// IsActiveKeyper checks if the given address is registered as a keyper in one of the active batch
// configs.
func (mainchain *MainChain) IsActiveKeyper(addr common.Address) bool {
	activeConfigs := mainchain.ActiveConfigs()
	for i := range activeConfigs {
		if activeConfigs[i].IsKeyper(addr) {
			return true
		}
	}
	return false
}

// ActiveConfigIndex returns the index of the config that is active for the given block number
func (mainchain *MainChain) ActiveConfigIndex(blocknum uint64) int {
	for i := len(mainchain.BatchConfigs) - 1; i >= 0; i-- {
		if mainchain.BatchConfigs[i].StartBlockNumber <= blocknum {
			return i
		}
	}
	panic("illegal values in MainChain.Configs field")
}

// ActiveConfig returns the config that is active for the given block number.
func (mainchain *MainChain) ActiveConfig(blocknum uint64) contract.BatchConfig {
	index := mainchain.ActiveConfigIndex(blocknum)
	return mainchain.BatchConfigs[index]
}

// CurrentConfig returns the batch config active at the current block number.
func (mainchain *MainChain) CurrentConfig() contract.BatchConfig {
	return mainchain.ActiveConfig(mainchain.CurrentBlock)
}

// ActiveConfigs returns a slice of the active configs.
func (mainchain *MainChain) ActiveConfigs() []contract.BatchConfig {
	return mainchain.BatchConfigs[mainchain.ActiveConfigIndex(mainchain.CurrentBlock):]
}

// ConfigIndexForBatchIndex returns the index of the config responsible for the given batch if it
// is active. Note that this config might change if the batch is too far into the future.
func (mainchain *MainChain) ConfigIndexForBatchIndex(batchIndex uint64) (int, bool) {
	for i := len(mainchain.BatchConfigs) - 1; i >= 0; i-- {
		config := mainchain.BatchConfigs[i]
		if config.StartBatchIndex <= batchIndex {
			if config.IsActive() {
				return i, true
			}
			return 0, false
		}
	}
	panic("illegal values in MainChain.Configs field")
}

// ConfigForBatchIndex returns the config responsible for the given batch if it is active. Note
// that this config might change if the batch is too far into the future.
func (mainchain *MainChain) ConfigForBatchIndex(batchIndex uint64) (contract.BatchConfig, bool) {
	index, ok := mainchain.ConfigIndexForBatchIndex(batchIndex)
	if !ok {
		return contract.BatchConfig{}, false
	}
	return mainchain.BatchConfigs[index], true
}

func (mainchain *MainChain) syncConfigs(cc *contract.Caller, opts *bind.CallOpts) error {
	numConfigs, err := cc.ConfigContract.NumConfigs(opts)
	if err != nil {
		return err
	}
	for configIndex := uint64(len(mainchain.BatchConfigs)); configIndex < numConfigs; configIndex++ {
		config, err := cc.ConfigContract.GetConfigByIndex(opts, configIndex)
		if err != nil {
			return err
		}
		mainchain.BatchConfigs = append(mainchain.BatchConfigs, config)
	}
	return nil
}

func (mainchain *MainChain) syncBatches(cc *contract.Caller, filter *bind.FilterOpts) error {
	it, err := cc.BatcherContract.FilterTransactionAdded(filter)
	if err != nil {
		return err
	}

	events := []*contract.BatcherContractTransactionAdded{}
	for it.Next() {
		events = append(events, it.Event)
	}
	if it.Error() != nil {
		return it.Error()
	}

	for _, event := range events {
		mainchain.addTransaction(event)
	}

	return nil
}

// AddTransaction adds a transaction to a batch according to a main chain TransactionAdded event.
func (mainchain *MainChain) addTransaction(event *contract.BatcherContractTransactionAdded) {
	batch, ok := mainchain.Batches[event.BatchIndex]
	if !ok {
		batch = &Batch{BatchIndex: event.BatchIndex}
		// for the rest of the fields, the zero values are fine
		mainchain.Batches[event.BatchIndex] = batch
	}

	switch event.TransactionType {
	case contract.TransactionTypeCipher:
		batch.EncryptedTransactions = append(batch.EncryptedTransactions, event.Transaction)
		batch.EncryptedBatchHash = event.BatchHash
	case contract.TransactionTypePlain:
		batch.PlainTransactions = append(batch.PlainTransactions, event.Transaction)
		batch.PlainBatchHash = event.BatchHash
	default:
		panic("unknown transaction type")
	}
}

func (mainchain *MainChain) syncExecutionState(cc *contract.Caller, opts *bind.CallOpts) error {
	lastNumExecutionHalfSteps := mainchain.NumExecutionHalfSteps

	numExecutionHalfSteps, err := cc.ExecutorContract.NumExecutionHalfSteps(opts)
	if err != nil {
		return err
	}

	receipts := []contract.CipherExecutionReceipt{}
	for i := lastNumExecutionHalfSteps; i < numExecutionHalfSteps; i++ {
		if i%2 != 0 {
			// there will only be receipts for cipher execution steps, which are the even ones
			continue
		}

		receipt, err := cc.ExecutorContract.CipherExecutionReceipts(opts, i)
		if err != nil {
			return err
		}
		receipts = append(receipts, receipt)
	}

	mainchain.NumExecutionHalfSteps = numExecutionHalfSteps
	for i := range receipts {
		mainchain.CipherExecutionReceipts[receipts[i].HalfStep] = &receipts[i]
	}

	return nil
}

func (mainchain *MainChain) syncDeposits(cc *contract.Caller, filter *bind.FilterOpts) error {
	eventIt, err := cc.DepositContract.FilterDepositChanged(filter, []common.Address{})
	if err != nil {
		return err
	}

	events := []*contract.DepositContractDepositChanged{}
	for eventIt.Next() {
		events = append(events, eventIt.Event)
	}
	if eventIt.Error() != nil {
		return eventIt.Error()
	}

	for _, ev := range events {
		deposit := mainchain.GetDeposit(ev.Account)
		deposit.Amount = ev.Amount
		deposit.WithdrawalDelayBlocks = ev.WithdrawalDelayBlocks
		deposit.WithdrawalRequestedBlock = ev.WithdrawalRequestedBlock
		deposit.Slashed = ev.Slashed
		mainchain.Deposits[ev.Account] = deposit
	}

	return nil
}

func (mainchain *MainChain) syncSlashings(cc *contract.Caller, filter *bind.FilterOpts) error {
	accusedIt, err := cc.KeyperSlasher.FilterAccused(filter, []uint64{}, []common.Address{}, []common.Address{})
	if err != nil {
		return err
	}
	appealedIt, err := cc.KeyperSlasher.FilterAppealed(filter, []uint64{}, []common.Address{})
	if err != nil {
		return err
	}
	// We don't filter for slashed events. Each slashing also results in a DepositChanged event in
	// the deposit contract which we already sync.

	accusedEvents := []*contract.KeyperSlasherAccused{}
	for accusedIt.Next() {
		accusedEvents = append(accusedEvents, accusedIt.Event)
	}
	if accusedIt.Error() != nil {
		return accusedIt.Error()
	}

	appealedEvents := []*contract.KeyperSlasherAppealed{}
	for appealedIt.Next() {
		appealedEvents = append(appealedEvents, appealedIt.Event)
	}
	if appealedIt.Error() != nil {
		return appealedIt.Error()
	}

	for _, ev := range accusedEvents {
		accusation := Accusation{
			Executor:    ev.Executor,
			Accuser:     ev.Accuser,
			Appealed:    false,
			HalfStep:    ev.HalfStep,
			BlockNumber: ev.Raw.BlockNumber,
		}
		mainchain.Accusations[accusation.HalfStep] = &accusation
	}
	for _, ev := range appealedEvents {
		accusation, ok := mainchain.Accusations[ev.HalfStep]
		if !ok {
			return errors.Errorf("got appeal without prior accusation: %+v", accusation)
		}
		accusation.Appealed = true
	}

	return nil
}

// GetDeposit returns the deposit of the given account or an empty one if it doesn't exist.
func (mainchain *MainChain) GetDeposit(account common.Address) *Deposit {
	deposit, ok := mainchain.Deposits[account]
	if !ok {
		deposit = &Deposit{
			Account: account,
		}
		mainchain.Deposits[account] = deposit
	}
	return deposit
}

// AccusationsAgainst returns all known accusations with the given account as executor.
func (mainchain *MainChain) AccusationsAgainst(account common.Address) []*Accusation {
	accusations := []*Accusation{}
	for _, a := range mainchain.Accusations {
		if a.Executor == account {
			accusations = append(accusations, a)
		}
	}
	return accusations
}

// SyncToHead fetches the latest state from the ethereum node. It returns a new object with the
// latest state.
func (mainchain *MainChain) SyncToHead(
	ctx context.Context,
	cc *contract.Caller,
) (*MainChain, error) {
	latestBlockHeader, err := cc.Ethclient.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}

	latestBlockNumber := latestBlockHeader.Number.Uint64()
	if latestBlockNumber == mainchain.CurrentBlock {
		return mainchain, nil
	}

	var syncUntilBlockNumber uint64
	if latestBlockNumber >= mainchain.FollowDistance {
		syncUntilBlockNumber = latestBlockNumber - mainchain.FollowDistance
	} else {
		syncUntilBlockNumber = 0
	}

	mainchain = mainchain.Clone()

	opts := &bind.CallOpts{
		BlockNumber: new(big.Int).SetUint64(syncUntilBlockNumber),
		Context:     ctx,
	}
	filter := &bind.FilterOpts{
		Start: mainchain.CurrentBlock + 1,
		End:   &syncUntilBlockNumber,
	}

	err = mainchain.syncConfigs(cc, opts)
	if err != nil {
		return nil, err
	}

	err = mainchain.syncBatches(cc, filter)
	if err != nil {
		return nil, err
	}

	err = mainchain.syncExecutionState(cc, opts)
	if err != nil {
		return nil, err
	}

	err = mainchain.syncDeposits(cc, filter)
	if err != nil {
		return nil, err
	}

	err = mainchain.syncSlashings(cc, filter)
	if err != nil {
		return nil, err
	}

	mainchain.CurrentBlock = syncUntilBlockNumber
	return mainchain, nil
}

// DecryptTransactions decrypts and shuffles the encrypted transactions. It will log an error
// message for transactions that cannot be decrypted and skip over them.
func (batch *Batch) DecryptTransactions(key *shcrypto.EpochSecretKey) [][]byte {
	var res [][]byte
	for idx, encTx := range batch.EncryptedTransactions {
		m := shcrypto.EncryptedMessage{}
		err := m.Unmarshal(encTx)
		if err != nil {
			log.Printf("Error: cannot unmarshal encrypted transaction #%d for batch index=%d: %s", idx, batch.BatchIndex, err)
			continue
		}
		decrypted, err := m.Decrypt(key)
		if err != nil {
			log.Printf("Error: cannot decrypt encrypted transaction #%d for batch index=%d: %s", idx, batch.BatchIndex, err)
			continue
		}
		res = append(res, decrypted)
	}
	return shcrypto.Shuffle(res, key)
}
