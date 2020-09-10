package keyper

import (
	"context"
	"crypto/ecdsa"
	"log"
	"time"

	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"
)

// NewKeyper creates a new Keyper
func NewKeyper(signingKey *ecdsa.PrivateKey, shuttermintURL string) Keyper {
	return Keyper{
		SigningKey:          signingKey,
		ShielderURL:      shuttermintURL,
		batchIndexToChannel: make(map[uint64]chan IEvent)}
}

// Run runs the keyper process. It determines the next BatchIndex and runs the key generation
// process for this BatchIndex and all following batches.

func (kpr *Keyper) Run() error {
	group, ctx := errgroup.WithContext(context.Background())
	var cl client.Client
	cl, err := http.New(kpr.ShielderURL, "/websocket")
	if err != nil {
		return err
	}

	err = cl.Start()

	if err != nil {
		return err
	}

	defer cl.Stop()
	query := "tx.height > 3"

	txs, err := cl.Subscribe(ctx, "test-client", query)
	kpr.txs = txs
	if err != nil {
		return err
	}

	group.Go(kpr.dispatchTxs)
	group.Go(kpr.startNewBatches)

	err = group.Wait()
	return err
}

func (kpr *Keyper) dispatchTxs() error {
	for tx := range kpr.txs {
		d := tx.Data.(types.EventDataTx)
		for _, ev := range d.TxResult.Result.Events {
			x, err := MakeEvent(ev)
			if err != nil {
				return err
			}
			kpr.dispatchEvent(x)
		}
	}
	return nil
}

func (kpr *Keyper) startNewBatches() error {
	var cl client.Client
	cl, err := http.New(kpr.ShielderURL, "/websocket")
	if err != nil {
		return err
	}

	for batchIndex := NextBatchIndex(time.Now()); ; batchIndex++ {
		bp := kpr.startBatch(batchIndex, cl)
		// The following waits for the start of the previous round. This is done on
		// purpose, because we generate keys in keyper.Run as a first step and then wait
		// for the start time
		SleepUntil(bp.PublicKeyGenerationStartTime)
	}
}

func (kpr *Keyper) removeBatch(batchIndex uint64) {
	log.Printf("Batch %d finished", batchIndex)
	kpr.mux.Lock()
	defer kpr.mux.Unlock()
	close(kpr.batchIndexToChannel[batchIndex])
	delete(kpr.batchIndexToChannel, batchIndex)
}

func (kpr *Keyper) startBatch(batchIndex uint64, cl client.Client) BatchParams {
	bp := NewBatchParams(batchIndex)
	ch := make(chan IEvent, 2)
	kpr.mux.Lock()
	defer kpr.mux.Unlock()

	kpr.batchIndexToChannel[batchIndex] = ch

	go func() {
		defer kpr.removeBatch(batchIndex)
		Run(bp, NewMessageSender(cl, kpr.SigningKey), ch)
	}()

	return bp
}

func (kpr *Keyper) dispatchEventToBatch(batchIndex uint64, ev IEvent) {
	kpr.mux.Lock()
	defer kpr.mux.Unlock()

	ch, ok := kpr.batchIndexToChannel[batchIndex]

	if ok {
		// We do have the mutex locked at this point and write to the channel.  This could
		// end up in a deadlock if we block here. This is why we need a buffered
		// channel.
		// Since we only ever close the channel, when the mutex is locked, at
		// least we can be sure that we do not write to a closed channel here.
		ch <- ev
	}
}

func (kpr *Keyper) dispatchEvent(ev IEvent) {
	log.Printf("Dispatching event: %+v", ev)
	switch e := ev.(type) {
	case PubkeyGeneratedEvent:
		kpr.dispatchEventToBatch(e.BatchIndex, e)
	case PrivkeyGeneratedEvent:
		kpr.dispatchEventToBatch(e.BatchIndex, e)
	case BatchConfigEvent:
		_ = e
	default:
		panic("unknown type")
	}
}
