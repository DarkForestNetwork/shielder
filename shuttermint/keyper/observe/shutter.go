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
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	pkgErrors "github.com/pkg/errors"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/rpc/client"
	rpctypes "github.com/tendermint/tendermint/rpc/core/types"

	"shielder/shuttermint/keyper/shielderevents"
	"shielder/shuttermint/medley"
)

const (
	shuttermintTimeout       = 10 * time.Second
	shielderReconnectInterval = 5 * time.Second
)

var errEonNotFound = errors.New("eon not found")

func init() {
	// Allow gob to serialize ecsda.PrivateKey and ed25519.PubKey
	gob.Register(ethcrypto.S256())
	gob.Register(ed25519.GenPrivKeyFromSecret([]byte{}).PubKey())
}

// EncryptionPublicKey is a gob serializable version of ecies.PublicKey.
type EncryptionPublicKey ecies.PublicKey

func (epk *EncryptionPublicKey) GobEncode() ([]byte, error) {
	return ethcrypto.FromECDSAPub((*ecies.PublicKey)(epk).ExportECDSA()), nil
}

func (epk *EncryptionPublicKey) GobDecode(data []byte) error {
	pubkey, err := ethcrypto.UnmarshalPubkey(data)
	if err != nil {
		return pkgErrors.Wrap(err, "failed to unmarshal encryption public key")
	}
	*epk = *(*EncryptionPublicKey)(ecies.ImportECDSAPublic(pubkey))
	return nil
}

// Encrypt the given message m.
func (epk *EncryptionPublicKey) Encrypt(rand io.Reader, m []byte) ([]byte, error) {
	return ecies.Encrypt(rand, (*ecies.PublicKey)(epk), m, nil, nil)
}

// Shielder let's a keyper fetch all necessary information from a shuttermint node. The only source
// for the data stored in this struct should be the shielder node.  The SyncToHead method can be
// used to update the data. All other accesses should be read-only.
type Shielder struct {
	CurrentBlock         int64
	LastCommittedHeight  int64
	NodeStatus           *rpctypes.ResultStatus
	KeyperEncryptionKeys map[common.Address]*EncryptionPublicKey
	BatchConfigs         []shielderevents.BatchConfig
	Batches              map[uint64]*BatchData
	Eons                 []Eon
}

// NewShielder creates an empty Shielder struct.
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
			log.Printf("Error: malformed event: %+v ev=%+v", err, ev)
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
	return nil, pkgErrors.WithStack(errEonNotFound)
}

func (shielder *Shielder) FindEon(eon uint64) (*Eon, error) {
	idx := shielder.searchEon(eon)
	if idx == len(shielder.Eons) || eon < shielder.Eons[idx].Eon {
		return nil, pkgErrors.WithStack(errEonNotFound)
	}
	return &shielder.Eons[idx], nil
}

func (shielder *Shielder) applyCheckIn(e shielderevents.CheckIn) error { //nolint:unparam
	shielder.KeyperEncryptionKeys[e.Sender] = (*EncryptionPublicKey)(e.EncryptionPublicKey)
	return nil
}

func (shielder *Shielder) applyBatchConfig(e shielderevents.BatchConfig) error { //nolint:unparam
	shielder.BatchConfigs = append(shielder.BatchConfigs, e)
	return nil
}

func (shielder *Shielder) applyDecryptionSignature(e shielderevents.DecryptionSignature) error { //nolint:unparam
	b := shielder.getBatchData(e.BatchIndex)
	b.DecryptionSignatures = append(b.DecryptionSignatures, e)
	return nil
}

func (shielder *Shielder) applyEonStarted(e shielderevents.EonStarted) error {
	idx := shielder.searchEon(e.Eon)
	if idx < len(shielder.Eons) {
		return pkgErrors.Errorf("eons should increase")
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
	case *shielderevents.CheckIn:
		err = shielder.applyCheckIn(*e)
	case *shielderevents.BatchConfig:
		err = shielder.applyBatchConfig(*e)
	case *shielderevents.DecryptionSignature:
		err = shielder.applyDecryptionSignature(*e)
	case *shielderevents.EonStarted:
		err = shielder.applyEonStarted(*e)
	case *shielderevents.PolyCommitment:
		err = shielder.applyPolyCommitment(*e)
	case *shielderevents.PolyEval:
		err = shielder.applyPolyEval(*e)
	case *shielderevents.Accusation:
		err = shielder.applyAccusation(*e)
	case *shielderevents.Apology:
		err = shielder.applyApology(*e)
	case *shielderevents.EpochSecretKeyShare:
		err = shielder.applyEpochSecretKeyShare(*e)
	default:
		err = pkgErrors.Errorf("not yet implemented for %s", reflect.TypeOf(ev))
	}
	if err != nil {
		log.Printf("Error in apply event: %+v, event: %+v", err, ev)
	}
}

func (shielder *Shielder) fetchAndApplyEvents(ctx context.Context, shmcl client.Client, targetHeight int64) (*Shielder, error) {
	if targetHeight < shielder.CurrentBlock {
		panic("internal error: fetchAndApplyEvents bad arguments")
	}
	query := fmt.Sprintf("tx.height >= %d and tx.height <= %d", shielder.CurrentBlock+1, targetHeight)

	// tendermint silently caps the perPage value at 100, make sure to stay below, otherwise
	// our exit condition is wrong and the log.Fatalf will trigger a panic below; see
	// https://shielder/issues/50
	perPage := 100
	page := 1
	total := 0
	cloned := false
	for {
		res, err := shmcl.TxSearch(ctx, query, false, &page, &perPage, "")
		if err != nil {
			return nil, pkgErrors.Wrap(err, "failed to fetch shuttermint txs")
		}
		// Create a shallow or deep clone
		if !cloned {
			if res.TotalCount == 0 {
				return shielder.ShallowClone(), nil
			}
			shielder = shielder.Clone()
			cloned = true
		}

		total += len(res.Txs)
		for _, tx := range res.Txs {
			events := tx.TxResult.GetEvents()
			shielder.applyTxEvents(tx.Height, events)
		}
		if page*perPage >= res.TotalCount {
			if total != res.TotalCount {
				log.Fatalf("internal error. got %d transactions, expected %d transactions from shuttermint for height %d..%d",
					total,
					res.TotalCount,
					shielder.CurrentBlock+1,
					targetHeight)
			}
			break
		}
		page++
	}
	return shielder, nil
}

// IsCheckedIn checks if the given address sent it's check-in message.
func (shielder *Shielder) IsCheckedIn(addr common.Address) bool {
	_, ok := shielder.KeyperEncryptionKeys[addr]
	return ok
}

// IsKeyper checks if the given address is a keyper in any of the given configs.
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
	return shielderevents.BatchConfig{}, pkgErrors.Errorf("cannot find BatchConfig with ConfigIndex==%d", configIndex)
}

func (shielder *Shielder) FindBatchConfigByBatchIndex(batchIndex uint64) shielderevents.BatchConfig {
	for i := len(shielder.BatchConfigs); i > 0; i++ {
		if shielder.BatchConfigs[i-1].StartBatchIndex <= batchIndex {
			return shielder.BatchConfigs[i-1]
		}
	}
	return shielderevents.BatchConfig{}
}

func (shielder *Shielder) ShallowClone() *Shielder {
	s := *shielder
	return &s
}

func (shielder *Shielder) Clone() *Shielder {
	clone := new(Shielder)
	medley.CloneWithGob(shielder, clone)
	return clone
}

func (shielder *Shielder) GetLastCommittedHeight(ctx context.Context, shmcl client.Client) (int64, error) {
	latestBlock, err := shmcl.Block(ctx, nil)
	if err != nil {
		return 0, pkgErrors.Wrap(err, "failed to get last committed height of shuttermint chain")
	}
	if latestBlock.Block == nil || latestBlock.Block.LastCommit == nil {
		return 0, pkgErrors.Errorf("empty blockchain: %+v", latestBlock)
	}
	return latestBlock.Block.LastCommit.Height, nil
}

// SyncToHead syncs the state with the remote state. It fetches events from new blocks since the
// last sync and updates the state by calling applyEvent for each event. This method does not
// mutate the object in place, it rather returns a new object.
func (shielder *Shielder) SyncToHead(ctx context.Context, shmcl client.Client) (*Shielder, error) {
	height, err := shielder.GetLastCommittedHeight(ctx, shmcl)
	if err != nil {
		return nil, err
	}
	if height == shielder.CurrentBlock {
		log.Printf("*** not syncing. already have synced to block %d", height)
		return shielder, nil
	}
	return shielder.SyncToHeight(ctx, shmcl, height)
}

// SyncToHeight syncs the state with the remote state until the given height.
func (shielder *Shielder) SyncToHeight(ctx context.Context, shmcl client.Client, height int64) (*Shielder, error) {
	nodeStatus, err := shmcl.Status(ctx)
	if err != nil {
		return nil, pkgErrors.Wrap(err, "failed to get shuttermint status")
	}

	lastCommittedHeight, err := shielder.GetLastCommittedHeight(ctx, shmcl)
	if err != nil {
		return nil, err
	}

	clone, err := shielder.fetchAndApplyEvents(ctx, shmcl, height)
	if err != nil {
		return nil, err
	}

	clone.CurrentBlock = height
	clone.LastCommittedHeight = lastCommittedHeight
	clone.NodeStatus = nodeStatus

	return clone, nil
}

// IsSynced checks if the shuttermint node is synced with the network.
func (shielder *Shielder) IsSynced() bool {
	return shielder.NodeStatus == nil || !shielder.NodeStatus.SyncInfo.CatchingUp
}

// SyncShielder subscribes to new blocks and syncs the shielder object with the head block in a
// loop. If writes newly synced shielder objects to the shielders channel, as well as errors to the
// syncErrors channel.
func SyncShielder(ctx context.Context, shmcl client.Client, shielder *Shielder, shielders chan<- *Shielder, syncErrors chan<- error) error {
	name := "keyper"
	query := "tm.event = 'NewBlock'"
	events, err := shmcl.Subscribe(ctx, name, query)
	if err != nil {
		return err
	}

	reconnect := func() {
		for {
			log.Println("Attempting reconnection to Shielder")

			ctx2, cancel2 := context.WithTimeout(ctx, shielderReconnectInterval)
			events, err = shmcl.Subscribe(ctx2, name, query)
			cancel2()

			if err != nil {
				// try again, unless context is canceled
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			} else {
				log.Println("Shielder connection regained")
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-events:
			newShielder, err := shielder.SyncToHead(ctx, shmcl)
			if err != nil {
				syncErrors <- err
			} else {
				shielders <- newShielder
				shielder = newShielder
			}
		case <-time.After(shuttermintTimeout):
			log.Println("No Shielder blocks received in a long time")
			reconnect()
		}
	}
}
