package observe

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/rpc/client"

	"shielder/shuttermint/keyper/shielderevents"
	"shielder/shuttermint/medley"
)

var errEonNotFound = errors.New("eon not found")

// Shielder let's a keyper fetch all necessary information from a shuttermint node. The only source
// for the data stored in this struct should be the shielder node.  The SyncToHead method can be
// used to update the data. All other accesses should be read-only.
type Shielder struct {
	CurrentBlock         int64
	KeyperEncryptionKeys map[common.Address]*ecies.PublicKey
	BatchConfigs         []shielderevents.BatchConfigEvent
	Batches              map[uint64]*Batch
	Eons                 []Eon
}

// NewShielder creates an empty Shielder struct
func NewShielder() *Shielder {
	return &Shielder{
		CurrentBlock:         -1,
		KeyperEncryptionKeys: make(map[common.Address]*ecies.PublicKey),
		Batches:              make(map[uint64]*Batch),
	}
}

type Eon struct {
	Eon         uint64
	StartEvent  shielderevents.EonStartedEvent
	Commitments []shielderevents.PolyCommitmentRegisteredEvent
	PolyEvals   []shielderevents.PolyEvalRegisteredEvent
}

type Batch struct {
	BatchIndex           uint64
	DecryptionSignatures []shielderevents.DecryptionSignatureEvent
}

func (shielder *Shielder) applyTxEvents(events []abcitypes.Event) {
	for _, ev := range events {
		x, err := shielderevents.MakeEvent(ev)
		if err != nil {
			fmt.Printf("malformed event: %+v", x)
		} else {
			shielder.applyEvent(x)
		}
	}
}

func (shielder *Shielder) getBatch(batchIndex uint64) *Batch {
	b, ok := shielder.Batches[batchIndex]
	if !ok {
		b = &Batch{BatchIndex: batchIndex}
		shielder.Batches[batchIndex] = b
	}
	return b
}

func (shielder *Shielder) findEon(eon uint64) (*Eon, error) {
	for i := 0; i < len(shielder.Eons); i++ {
		if shielder.Eons[i].Eon == eon {
			return &shielder.Eons[i], nil
		}
	}
	return nil, errEonNotFound
}

func (shielder *Shielder) applyEvent(ev shielderevents.IEvent) {
	warn := func() {
		fmt.Printf("XXX observing event not yet implemented: %s%+v\n", reflect.TypeOf(ev), ev)
	}
	switch e := ev.(type) {
	case shielderevents.CheckInEvent:
		shielder.KeyperEncryptionKeys[e.Sender] = e.EncryptionPublicKey
	case shielderevents.BatchConfigEvent:
		shielder.BatchConfigs = append(shielder.BatchConfigs, e)
	case shielderevents.DecryptionSignatureEvent:
		b := shielder.getBatch(e.BatchIndex)
		b.DecryptionSignatures = append(b.DecryptionSignatures, e)
	case shielderevents.EonStartedEvent:
		_, err := shielder.findEon(e.Eon)
		if err == nil {
			panic("duplicate EonStartedEvent received")
		}
		shielder.Eons = append(shielder.Eons, Eon{Eon: e.Eon, StartEvent: e})
	case shielderevents.PolyCommitmentRegisteredEvent:
		eon, err := shielder.findEon(e.Eon)
		if err != nil {
			panic(err) // XXX we should remove that later
		}
		eon.Commitments = append(eon.Commitments, e)
	case shielderevents.PolyEvalRegisteredEvent:
		eon, err := shielder.findEon(e.Eon)
		if err != nil {
			panic(err) // XXX we should remove that later
		}
		eon.PolyEvals = append(eon.PolyEvals, e)
	default:
		warn()
		panic("applyEvent: unknown event. giving up")
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
			shielder.applyTxEvents(events)
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
		_, err := medley.FindAddressIndex(cfg.Keypers, addr)
		if err == nil {
			return true
		}
	}
	return false
}

func (shielder *Shielder) FindBatchConfigByBatchIndex(batchIndex uint64) shielderevents.BatchConfigEvent {
	for i := len(shielder.BatchConfigs); i > 0; i++ {
		if shielder.BatchConfigs[i-1].StartBatchIndex <= batchIndex {
			return shielder.BatchConfigs[i-1]
		}
	}
	return shielderevents.BatchConfigEvent{}
}

// SyncToHead syncs the state with the remote state. It fetches events from new blocks since the
// last sync and updates the state by calling applyEvent for each event.
// XXX this mutates the object in place. we may want to control mutation of the Shielder struct.
func (shielder *Shielder) SyncToHead(ctx context.Context, shmcl client.Client) error {
	latestBlock, err := shmcl.Block(ctx, nil)
	if err != nil {
		return err
	}

	err = shielder.fetchAndApplyEvents(ctx, shmcl, latestBlock.Block.Header.Height)
	if err != nil {
		return err
	}
	shielder.CurrentBlock = latestBlock.Block.Header.Height
	return nil
}
