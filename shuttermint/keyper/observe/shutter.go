package observe

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/rpc/client"

	"shielder/shuttermint/keyper/shielderevents"
	"shielder/shuttermint/medley"
)

var errEonNotFound = errors.New("eon not found")

func init() {
	gob.Register(ethcrypto.S256()) // Allow gob to serialize ecsda.PrivateKey
}

// EncryptionPublicKey is a gob serializable version of ecies.PublicKey
type EncryptionPublicKey ecies.PublicKey

func (epk *EncryptionPublicKey) GobEncode() ([]byte, error) {
	return ethcrypto.FromECDSAPub((*ecies.PublicKey)(epk).ExportECDSA()), nil
}

func (epk *EncryptionPublicKey) GobDecode(data []byte) error {
	pubkey, err := ethcrypto.UnmarshalPubkey(data)
	if err != nil {
		return err
	}
	*epk = *(*EncryptionPublicKey)(ecies.ImportECDSAPublic(pubkey))
	return nil
}

// Encrypt the given message m
func (epk *EncryptionPublicKey) Encrypt(rand io.Reader, m []byte) ([]byte, error) {
	return ecies.Encrypt(rand, (*ecies.PublicKey)(epk), m, nil, nil)
}

// Shielder let's a keyper fetch all necessary information from a shuttermint node. The only source
// for the data stored in this struct should be the shielder node.  The SyncToHead method can be
// used to update the data. All other accesses should be read-only.
type Shielder struct {
	CurrentBlock         int64
	KeyperEncryptionKeys map[common.Address]*EncryptionPublicKey
	BatchConfigs         []shielderevents.BatchConfig
	Batches              map[uint64]*BatchData
	Eons                 []Eon
}

// NewShielder creates an empty Shielder struct
func NewShielder() *Shielder {
	return &Shielder{
		CurrentBlock:         -1,
		KeyperEncryptionKeys: make(map[common.Address]*EncryptionPublicKey),
		Batches:              make(map[uint64]*BatchData),
	}
}

type Eon struct {
	Eon                  uint64
	StartHeight          int64
	StartEvent           shielderevents.EonStarted
	Commitments          []shielderevents.PolyCommitment
	PolyEvals            []shielderevents.PolyEval
	Accusations          []shielderevents.Accusation
	Apologies            []shielderevents.Apology
	EpochSecretKeyShares []shielderevents.EpochSecretKeyShare
}

type BatchData struct {
	BatchIndex           uint64
	DecryptionSignatures []shielderevents.DecryptionSignature
}

func (shielder *Shielder) applyTxEvents(height int64, events []abcitypes.Event) {
	for _, ev := range events {
		x, err := shielderevents.MakeEvent(ev, height)
		if err != nil {
			log.Printf("Error: malformed event: %s ev=%+v", err, ev)
		} else {
			shielder.applyEvent(x)
		}
	}
}

func (shielder *Shielder) getBatchData(batchIndex uint64) *BatchData {
	b, ok := shielder.Batches[batchIndex]
	if !ok {
		b = &BatchData{BatchIndex: batchIndex}
		shielder.Batches[batchIndex] = b
	}
	return b
}

func (shielder *Shielder) searchEon(eon uint64) int {
	return sort.Search(
		len(shielder.Eons),
		func(i int) bool {
			return eon <= shielder.Eons[i].Eon
		},
	)
}

func (shielder *Shielder) FindEonByBatchIndex(batchIndex uint64) (*Eon, error) {
	for i := len(shielder.Eons) - 1; i >= 0; i-- {
		if shielder.Eons[i].StartEvent.BatchIndex <= batchIndex {
			return &shielder.Eons[i], nil
		}
	}
	return nil, errEonNotFound
}

func (shielder *Shielder) FindEon(eon uint64) (*Eon, error) {
	idx := shielder.searchEon(eon)
	if idx == len(shielder.Eons) || eon < shielder.Eons[idx].Eon {
		return nil, errEonNotFound
	}
	return &shielder.Eons[idx], nil
}

func (shielder *Shielder) applyCheckIn(e shielderevents.CheckIn) error {
	shielder.KeyperEncryptionKeys[e.Sender] = (*EncryptionPublicKey)(e.EncryptionPublicKey)
	return nil
}

func (shielder *Shielder) applyBatchConfig(e shielderevents.BatchConfig) error {
	shielder.BatchConfigs = append(shielder.BatchConfigs, e)
	return nil
}

func (shielder *Shielder) applyDecryptionSignature(e shielderevents.DecryptionSignature) error {
	b := shielder.getBatchData(e.BatchIndex)
	b.DecryptionSignatures = append(b.DecryptionSignatures, e)
	return nil
}

func (shielder *Shielder) applyEonStarted(e shielderevents.EonStarted) error {
	idx := shielder.searchEon(e.Eon)
	if idx < len(shielder.Eons) {
		return fmt.Errorf("eons should increase")
	}
	shielder.Eons = append(shielder.Eons, Eon{Eon: e.Eon, StartEvent: e, StartHeight: e.Height})
	return nil
}

func (shielder *Shielder) applyPolyCommitment(e shielderevents.PolyCommitment) error {
	eon, err := shielder.FindEon(e.Eon)
	if err != nil {
		return err
	}
	eon.Commitments = append(eon.Commitments, e)
	return nil
}

func (shielder *Shielder) applyPolyEval(e shielderevents.PolyEval) error {
	eon, err := shielder.FindEon(e.Eon)
	if err != nil {
		return err
	}
	eon.PolyEvals = append(eon.PolyEvals, e)
	return nil
}

func (shielder *Shielder) applyAccusation(e shielderevents.Accusation) error {
	eon, err := shielder.FindEon(e.Eon)
	if err != nil {
		return err
	}
	eon.Accusations = append(eon.Accusations, e)
	return nil
}

func (shielder *Shielder) applyApology(e shielderevents.Apology) error {
	eon, err := shielder.FindEon(e.Eon)
	if err != nil {
		return err
	}
	eon.Apologies = append(eon.Apologies, e)
	return nil
}

func (shielder *Shielder) applyEpochSecretKeyShare(e shielderevents.EpochSecretKeyShare) error {
	eon, err := shielder.FindEon(e.Eon)
	if err != nil {
		return err
	}
	eon.EpochSecretKeyShares = append(eon.EpochSecretKeyShares, e)
	return nil
}

func (shielder *Shielder) applyEvent(ev shielderevents.IEvent) {
	var err error
	switch e := ev.(type) {
	case shielderevents.CheckIn:
		err = shielder.applyCheckIn(e)
	case shielderevents.BatchConfig:
		err = shielder.applyBatchConfig(e)
	case shielderevents.DecryptionSignature:
		err = shielder.applyDecryptionSignature(e)
	case shielderevents.EonStarted:
		err = shielder.applyEonStarted(e)
	case shielderevents.PolyCommitment:
		err = shielder.applyPolyCommitment(e)
	case shielderevents.PolyEval:
		err = shielder.applyPolyEval(e)
	case shielderevents.Accusation:
		err = shielder.applyAccusation(e)
	case shielderevents.Apology:
		err = shielder.applyApology(e)
	case shielderevents.EpochSecretKeyShare:
		err = shielder.applyEpochSecretKeyShare(e)
	default:
		err = fmt.Errorf("not yet implemented for %s", reflect.TypeOf(ev))
	}
	if err != nil {
		log.Printf("Error in apply event: %s, event: %+v", err, ev)
	}
}

func (shielder *Shielder) fetchAndApplyEvents(ctx context.Context, shmcl client.Client, targetHeight int64) error {
	if targetHeight < shielder.CurrentBlock {
		panic("internal error: fetchAndApplyEvents bad arguments")
	}
	query := fmt.Sprintf("tx.height >= %d and tx.height<=%d", shielder.CurrentBlock+1, targetHeight)

	page := 1
	perPage := 200
	for {
		res, err := shmcl.TxSearch(ctx, query, false, &page, &perPage, "")
		if err != nil {
			return err
		}
		for _, tx := range res.Txs {
			events := tx.TxResult.GetEvents()
			shielder.applyTxEvents(tx.Height, events)
		}
		if page*perPage > res.TotalCount {
			break
		}
		page++
	}
	return nil
}

// IsCheckedIn checks if the given address sent it's checkin message
func (shielder *Shielder) IsCheckedIn(addr common.Address) bool {
	_, ok := shielder.KeyperEncryptionKeys[addr]
	return ok
}

// IsKeyper checks if the given address is a keyper in any of the given configs
func (shielder *Shielder) IsKeyper(addr common.Address) bool {
	for _, cfg := range shielder.BatchConfigs {
		if cfg.IsKeyper(addr) {
			return true
		}
	}
	return false
}

func (shielder *Shielder) FindBatchConfigByConfigIndex(configIndex uint64) (shielderevents.BatchConfig, error) {
	for _, bc := range shielder.BatchConfigs {
		if bc.ConfigIndex == configIndex {
			return bc, nil
		}
	}
	return shielderevents.BatchConfig{}, fmt.Errorf("cannot find BatchConfig with ConfigIndex==%d", configIndex)
}

func (shielder *Shielder) FindBatchConfigByBatchIndex(batchIndex uint64) shielderevents.BatchConfig {
	for i := len(shielder.BatchConfigs); i > 0; i++ {
		if shielder.BatchConfigs[i-1].StartBatchIndex <= batchIndex {
			return shielder.BatchConfigs[i-1]
		}
	}
	return shielderevents.BatchConfig{}
}

func (shielder *Shielder) Clone() *Shielder {
	clone := new(Shielder)
	medley.CloneWithGob(shielder, clone)
	return clone
}

func (shielder *Shielder) LastCommittedHeight(ctx context.Context, shmcl client.Client) (int64, error) {
	latestBlock, err := shmcl.Block(ctx, nil)
	if err != nil {
		return 0, err
	}
	if latestBlock.Block == nil || latestBlock.Block.LastCommit == nil {
		return 0, fmt.Errorf("empty blockchain: %+v", latestBlock)
	}
	return latestBlock.Block.LastCommit.Height, nil
}

// SyncToHead syncs the state with the remote state. It fetches events from new blocks since the
// last sync and updates the state by calling applyEvent for each event. This method does not
// mutate the object in place, it rather returns a new object.
func (shielder *Shielder) SyncToHead(ctx context.Context, shmcl client.Client) (*Shielder, error) {
	height, err := shielder.LastCommittedHeight(ctx, shmcl)
	if err != nil {
		return nil, err
	}
	return shielder.SyncToHeight(ctx, shmcl, height)
}

// SyncToHeight syncs the state with the remote state until the given height
func (shielder *Shielder) SyncToHeight(ctx context.Context, shmcl client.Client, height int64) (*Shielder, error) {
	clone := shielder.Clone()
	err := clone.fetchAndApplyEvents(ctx, shmcl, height)
	if err != nil {
		return nil, err
	}
	clone.CurrentBlock = height
	return clone, nil
}
