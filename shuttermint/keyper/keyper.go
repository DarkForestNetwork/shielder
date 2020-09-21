package keyper

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	tmtypes "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"

	"shielder/shuttermint/contract"
)

const newHeadersSize = 8

// NewKeyper creates a new Keyper
func NewKeyper(signingKey *ecdsa.PrivateKey, shuttermintURL string, ethereumURL string) Keyper {
	return Keyper{
		SigningKey:     signingKey,
		ShielderURL: shuttermintURL,
		EthereumURL:    ethereumURL,
		batches:        make(map[uint64]*BatchState),
		newHeaders:     make(chan *types.Header, newHeadersSize),
	}
}

func (kpr *Keyper) init() error {
	ethcl, err := ethclient.Dial(kpr.EthereumURL)
	if err != nil {
		return err
	}
	kpr.ethcl = ethcl
	group, ctx := errgroup.WithContext(context.Background())
	kpr.ctx = ctx
	kpr.group = group
	addr := common.HexToAddress("0x07a457d878BF363E0Bb5aa0B096092f941e19962")
	cc, err := contract.NewConfigContract(addr, kpr.ethcl)
	if err != nil {
		return err
	}
	kpr.configContract = cc
	return nil
}

// Run runs the keyper process. It determines the next BatchIndex and runs the key generation
// process for this BatchIndex and all following batches.
func (kpr *Keyper) Run() error {
	log.Printf("Running keyper with address %s", kpr.Address().Hex())
	err := kpr.init()
	if err != nil {
		return err
	}

	var shmcl client.Client
	shmcl, err = http.New(kpr.ShielderURL, "/websocket")
	if err != nil {
		return errors.Wrapf(err, "create shuttermint client at %s", kpr.ShielderURL)
	}

	err = shmcl.Start()

	if err != nil {
		return errors.Wrapf(err, "start shuttermint client at %s", kpr.ShielderURL)
	}

	defer shmcl.Stop()
	query := "tx.height > 3"

	txs, err := shmcl.Subscribe(kpr.ctx, "test-client", query)
	kpr.txs = txs
	if err != nil {
		return err
	}

	kpr.group.Go(kpr.dispatchTxs)
	kpr.group.Go(kpr.startNewBatches)
	kpr.group.Go(kpr.watchMainChainHeadBlock)
	err = kpr.group.Wait()
	return err
}

// IsWebsocketURL returns true iff the given URL is a websocket URL, i.e. if it starts with ws://
// or wss://. This is needed for the watchMainChainHeadBlock method.
func IsWebsocketURL(url string) bool {
	return strings.HasPrefix(url, "ws://") || strings.HasPrefix(url, "wss://")
}

func (kpr *Keyper) watchMainChainHeadBlock() error {
	if !IsWebsocketURL(kpr.EthereumURL) {
		err := fmt.Errorf("must use ws:// or wss:// URL, have %s", kpr.EthereumURL)
		log.Printf("Error: %s", err)
		return err
	}

	cl, err := ethclient.Dial(kpr.EthereumURL)
	if err != nil {
		return err
	}
	headers := make(chan *types.Header)

	subscription, err := cl.SubscribeNewHead(kpr.ctx, headers)
	if err != nil {
		return err
	}
	for {
		select {
		case <-kpr.ctx.Done():
			return nil
		case err := <-subscription.Err():
			return err
		case header := <-headers:
			kpr.newHeaders <- header
			kpr.dispatchNewBlockHeader(header)
		}
	}
}

func (kpr *Keyper) dispatchNewBlockHeader(header *types.Header) {
	log.Printf("Dispatching new block #%d to %d batches\n", header.Number, len(kpr.batches))
	kpr.mux.Lock()
	defer kpr.mux.Unlock()

	for _, batch := range kpr.batches {
		batch.NewBlockHeader(header)
	}
}

func (kpr *Keyper) dispatchTxs() error {
	// Unfortunately the channel isn't being closed for us, so we manually check if we should
	// cancel
	for {
		select {
		case tx := <-kpr.txs:
			d := tx.Data.(tmtypes.EventDataTx)
			for _, ev := range d.TxResult.Result.Events {
				x, err := MakeEvent(ev)
				if err != nil {
					return err
				}
				kpr.dispatchEvent(x)
			}
		case <-kpr.ctx.Done():
			return nil
		}
	}
}

// waitCurrentBlockHeaders waits for a block header that is at max 1 Minute in the past. We do want
// to ignore block headers we may get when syncing.
func (kpr *Keyper) waitCurrentBlockHeader() *types.Header {
	for {
		select {
		case <-kpr.ctx.Done():
			return nil
		case header := <-kpr.newHeaders:
			if time.Since(time.Unix(int64(header.Time), 0)) < time.Minute {
				return header
			}
		}
	}
}

func (kpr *Keyper) startNewBatches() error {
	var cl client.Client
	cl, err := http.New(kpr.ShielderURL, "/websocket")
	if err != nil {
		return err
	}
	_ = cl

	// We wait for block header that is not too old.

	header := kpr.waitCurrentBlockHeader()
	if header == nil {
		return nil
	}

	nextBatchIndex, err := kpr.configContract.NextBatchIndex(header.Number.Uint64())
	if err != nil {
		return err
	}
	lastBatchIndex := nextBatchIndex - 1 // we'll start only the batch for nextBatchIndex
	for {
		for batchIndex := lastBatchIndex + 1; batchIndex <= nextBatchIndex; batchIndex++ {
			log.Printf("Starting batch #%d", batchIndex)
			err = kpr.startBatch(nextBatchIndex, cl)
			if err != nil {
				log.Printf("Error: could not start batch #%d: %s", nextBatchIndex, err)
			}
		}
		lastBatchIndex = nextBatchIndex

		select {
		case <-kpr.ctx.Done():
			return nil
		case header := <-kpr.newHeaders:
			nextBatchIndex, err = kpr.configContract.NextBatchIndex(header.Number.Uint64())
			if err != nil {
				return err
			}
		}
	}
}

func (kpr *Keyper) removeBatch(batchIndex uint64) {
	log.Printf("Batch %d finished", batchIndex)
	kpr.mux.Lock()
	defer kpr.mux.Unlock()
	delete(kpr.batches, batchIndex)
}

func (kpr *Keyper) startBatch(batchIndex uint64, cl client.Client) error {
	bp, err := kpr.configContract.QueryBatchParams(nil, batchIndex)
	if err != nil {
		return err
	}

	ms := NewMessageSender(cl, kpr.SigningKey)
	cc := NewContractCaller(
		kpr.EthereumURL,
		kpr.SigningKey,
		common.HexToAddress("0x791c3f20f865c582A204134E0A64030Fc22D2E38"),
	)
	batch := NewBatchState(bp, kpr.SigningKey, &ms, &cc)

	kpr.mux.Lock()
	defer kpr.mux.Unlock()

	kpr.batches[batchIndex] = &batch

	go func() {
		defer kpr.removeBatch(batchIndex)

		// XXX we should check against what is configured in shuttermint
		// Here is the code we used to get the config from shuttermint:

		// bc, err := queryBatchConfig(cl, batchIndex)
		// if err != nil {
		//	log.Print("Error while trying to query batch config:", err)
		//	return
		// }
		// batch.BatchParams.BatchConfig = bc

		batch.Run()
	}()

	return nil
}

func (kpr *Keyper) dispatchEventToBatch(batchIndex uint64, ev IEvent) {
	kpr.mux.Lock()
	defer kpr.mux.Unlock()

	batch, ok := kpr.batches[batchIndex]

	if ok {
		batch.dispatchShielderEvent(ev)
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
	case EncryptionKeySignatureAddedEvent:
		kpr.dispatchEventToBatch(e.BatchIndex, e)
	default:
		panic("unknown type")
	}
}

// Address returns the keyper's Ethereum address.
func (kpr *Keyper) Address() common.Address {
	return crypto.PubkeyToAddress(kpr.SigningKey.PublicKey)
}
