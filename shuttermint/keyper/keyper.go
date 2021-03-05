// Package keyper contains the keyper implementation
package keyper

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/kr/pretty"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"golang.org/x/sync/errgroup"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/fx"
	"shielder/shuttermint/keyper/observe"
)

const (
	// mainChainTimeout is the time after which we assume the connection to the main chain
	// node is lost if no new block is received
	mainChainTimeout           = 30 * time.Second
	mainChainReconnectInterval = 5 * time.Second // time between two reconnection attempts
	shuttermintTimeout         = 10 * time.Second
	shielderReconnectInterval   = 5 * time.Second
)

// IsWebsocketURL returns true iff the given URL is a websocket URL, i.e. if it starts with ws://
// or wss://. This is needed for the watchMainChainHeadBlock method.
func IsWebsocketURL(url string) bool {
	return strings.HasPrefix(url, "ws://") || strings.HasPrefix(url, "wss://")
}

// Address returns the keyper's Ethereum address.
func (c *KeyperConfig) Address() common.Address {
	return crypto.PubkeyToAddress(c.SigningKey.PublicKey)
}

type Keyper struct {
	Config    KeyperConfig
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain

	ContractCaller contract.Caller
	shmcl          client.Client
	MessageSender  fx.MessageSender
	lastlogTime    time.Time
	runenv         *fx.RunEnv
}

func NewKeyper(kc KeyperConfig) Keyper {
	return Keyper{
		Config:    kc,
		State:     NewState(),
		Shielder:   observe.NewShielder(),
		MainChain: observe.NewMainChain(),
	}
}

func NewContractCallerFromConfig(config KeyperConfig) (contract.Caller, error) {
	ethcl, err := ethclient.Dial(config.EthereumURL)
	if err != nil {
		return contract.Caller{}, err
	}
	configContract, err := contract.NewConfigContract(config.ConfigContractAddress, ethcl)
	if err != nil {
		return contract.Caller{}, err
	}

	keyBroadcastContract, err := contract.NewKeyBroadcastContract(config.KeyBroadcastContractAddress, ethcl)
	if err != nil {
		return contract.Caller{}, err
	}

	batcherContract, err := contract.NewBatcherContract(config.BatcherContractAddress, ethcl)
	if err != nil {
		return contract.Caller{}, err
	}

	executorContract, err := contract.NewExecutorContract(config.ExecutorContractAddress, ethcl)
	if err != nil {
		return contract.Caller{}, err
	}

	depositContract, err := contract.NewDepositContract(config.DepositContractAddress, ethcl)
	if err != nil {
		return contract.Caller{}, err
	}

	keyperSlasher, err := contract.NewKeyperSlasher(config.KeyperSlasherAddress, ethcl)
	if err != nil {
		return contract.Caller{}, err
	}

	return contract.NewContractCaller(
		ethcl,
		config.SigningKey,
		configContract,
		keyBroadcastContract,
		batcherContract,
		executorContract,
		depositContract,
		keyperSlasher,
	), nil
}

func (kpr *Keyper) init() error {
	if kpr.shmcl != nil {
		panic("internal error: already initialized")
	}
	var err error
	kpr.shmcl, err = http.New(kpr.Config.ShielderURL, "/websocket")
	if err != nil {
		return errors.Wrapf(err, "create shuttermint client at %s", kpr.Config.ShielderURL)
	}
	err = kpr.shmcl.Start()
	if err != nil {
		return errors.Wrapf(err, "start shuttermint client")
	}
	ms := fx.NewRPCMessageSender(kpr.shmcl, kpr.Config.SigningKey)
	kpr.MessageSender = &ms

	kpr.ContractCaller, err = NewContractCallerFromConfig(kpr.Config)
	kpr.runenv = fx.NewRunEnv(kpr.MessageSender, &kpr.ContractCaller)
	return err
}

func (kpr *Keyper) syncMain(ctx context.Context, mainChains chan<- *observe.MainChain, syncErrors chan<- error) error {
	headers := make(chan *types.Header)
	sub, err := kpr.ContractCaller.Ethclient.SubscribeNewHead(ctx, headers)
	if err != nil {
		return err
	}

	reconnect := func() {
		sub.Unsubscribe()
		for {
			log.Println("Attempting reconnection to main chain")
			sub, err = kpr.ContractCaller.Ethclient.SubscribeNewHead(ctx, headers)
			if err != nil {
				select {
				case <-time.After(mainChainReconnectInterval):
					continue
				case <-ctx.Done():
					return
				}
			} else {
				log.Println("Main chain connection regained")
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return nil
		case <-headers:
			newMainChain, err := kpr.MainChain.SyncToHead(ctx, &kpr.ContractCaller)
			if err != nil {
				syncErrors <- err
			} else {
				mainChains <- newMainChain
			}
		case err := <-sub.Err():
			log.Println("Main chain connection lost:", err)
			reconnect()
		case <-time.After(mainChainTimeout):
			log.Println("No main chain blocks received in a long time")
			reconnect()
		}
	}
}

func (kpr *Keyper) syncShielder(ctx context.Context, shielders chan<- *observe.Shielder, syncErrors chan<- error) error {
	name := "keyper"
	query := "tm.event = 'NewBlock'"
	events, err := kpr.shmcl.Subscribe(ctx, name, query)
	if err != nil {
		return err
	}

	reconnect := func() {
		for {
			log.Println("Attempting reconnection to Shielder")

			ctx2, cancel2 := context.WithTimeout(ctx, shielderReconnectInterval)
			events, err = kpr.shmcl.Subscribe(ctx2, name, query)
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
			newShielder, err := kpr.Shielder.SyncToHead(ctx, kpr.shmcl)
			if err != nil {
				syncErrors <- err
			} else {
				shielders <- newShielder
			}
		case <-time.After(shuttermintTimeout):
			log.Println("No Shielder blocks received in a long time")
			reconnect()
		}
	}
}

func (kpr *Keyper) ShortInfo() string {
	var dkgInfo []string
	for _, dkg := range kpr.State.DKGs {
		dkgInfo = append(dkgInfo, dkg.ShortInfo())
	}
	return fmt.Sprintf(
		"shielder block %d, main chain %d, last eon started %d, num half steps: %d, DKGs: %s",
		kpr.Shielder.CurrentBlock,
		kpr.MainChain.CurrentBlock,
		kpr.State.LastEonStarted,
		kpr.MainChain.NumExecutionHalfSteps,
		strings.Join(dkgInfo, " - "),
	)
}

func (kpr *Keyper) syncOnce(ctx context.Context) error {
	newMain, err := kpr.MainChain.SyncToHead(ctx, &kpr.ContractCaller)
	if err != nil {
		return err
	}
	kpr.MainChain = newMain

	newShielder, err := kpr.Shielder.SyncToHead(ctx, kpr.shmcl)
	if err != nil {
		return err
	}
	kpr.Shielder = newShielder

	return nil
}

func (kpr *Keyper) Run() error {
	err := kpr.init()
	if err != nil {
		return err
	}
	g, ctx := errgroup.WithContext(context.Background())

	// Sync main and shielder chain once. Otherwise, the state of one of the two will be much more
	// recent than the other one when the first block appears.
	err = kpr.syncOnce(ctx)
	if err != nil {
		return err
	}

	mainChains := make(chan *observe.MainChain)
	shielders := make(chan *observe.Shielder)
	syncErrors := make(chan error)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)
	g.Go(func() error { return kpr.syncMain(ctx, mainChains, syncErrors) })
	g.Go(func() error { return kpr.syncShielder(ctx, shielders, syncErrors) })
	kpr.runenv.StartBackgroundTasks(ctx, g)

	for {
		select {
		case sig := <-signals:
			log.Printf("Received %s. Dumping internal state", sig)
			pretty.Println("Shielder:", kpr.Shielder)
			pretty.Println("Mainchain:", kpr.MainChain)
			pretty.Println("State:", kpr.State)
		case mainChain := <-mainChains:
			kpr.MainChain = mainChain
			kpr.runOneStep(ctx)
		case shielder := <-shielders:
			kpr.Shielder = shielder
			kpr.runOneStep(ctx)
		case err := <-syncErrors:
			return err
		case <-ctx.Done():
			return g.Wait()
		}
	}
}

type storedState struct {
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain
}

func (kpr *Keyper) gobpath() string {
	return filepath.Join(kpr.Config.DBDir, "state.gob")
}

func (kpr *Keyper) LoadState() error {
	gobpath := kpr.gobpath()

	gobfile, err := os.Open(gobpath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	log.Printf("Loading state from %s", gobpath)

	defer gobfile.Close()
	dec := gob.NewDecoder(gobfile)
	st := storedState{}
	err = dec.Decode(&st)
	if err != nil {
		return err
	}
	kpr.State = st.State
	kpr.Shielder = st.Shielder
	kpr.MainChain = st.MainChain
	return nil
}

func (kpr *Keyper) saveState() error {
	gobpath := kpr.gobpath()
	tmppath := gobpath + ".tmp"
	file, err := os.Create(tmppath)
	if err != nil {
		return err
	}
	defer file.Close()
	st := storedState{
		State:     kpr.State,
		Shielder:   kpr.Shielder,
		MainChain: kpr.MainChain,
	}
	enc := gob.NewEncoder(file)
	err = enc.Encode(st)
	if err != nil {
		return err
	}

	err = file.Sync()
	if err != nil {
		return err
	}
	err = os.Rename(tmppath, gobpath)
	return err
}

func (kpr *Keyper) runOneStep(ctx context.Context) {
	decider := NewDecider(kpr)
	decider.Decide()

	now := time.Now()
	if len(decider.Actions) > 0 || now.Sub(kpr.lastlogTime) > 10*time.Second {
		log.Println(kpr.ShortInfo())
		kpr.lastlogTime = now
	}

	err := kpr.saveState()
	if err != nil {
		panic(err)
	}

	kpr.runenv.RunActions(ctx, decider.Actions)
}
