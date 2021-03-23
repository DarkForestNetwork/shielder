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
		MainChain: observe.NewMainChain(kc.MainChainFollowDistance),
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
	if err != nil {
		return err
	}
	kpr.runenv = fx.NewRunEnv(kpr.MessageSender, &kpr.ContractCaller, kpr.CurrentWorld, kpr.pathActionsGob())
	return nil
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
	g.Go(func() error {
		return observe.SyncMain(ctx, &kpr.ContractCaller, kpr.MainChain, mainChains, syncErrors)
	})
	g.Go(func() error {
		return observe.SyncShielder(ctx, kpr.shmcl, kpr.Shielder, shielders, syncErrors)
	})
	kpr.runenv.StartBackgroundTasks(ctx, g)
	err = kpr.runenv.Load()
	if err != nil {
		return err
	}

	if len(kpr.State.Actions) > 0 {
		kpr.runActions(ctx)
	}

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

func (kpr *Keyper) CurrentWorld() observe.World {
	return observe.World{Shielder: kpr.Shielder, MainChain: kpr.MainChain}
}

type storedState struct {
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain
	Actions   []fx.IAction
}

func (kpr *Keyper) pathStateGob() string {
	return filepath.Join(kpr.Config.DBDir, "state.gob")
}

func (kpr *Keyper) pathActionsGob() string {
	return filepath.Join(kpr.Config.DBDir, "actions.gob")
}

func (kpr *Keyper) LoadState() error {
	gobpath := kpr.pathStateGob()

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
	gobpath := kpr.pathStateGob()
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

func (kpr *Keyper) runActions(ctx context.Context) {
	now := time.Now()
	if len(kpr.State.Actions) > 0 || now.Sub(kpr.lastlogTime) > 10*time.Second {
		log.Println(kpr.ShortInfo())
		kpr.lastlogTime = now
	}

	kpr.runenv.RunActions(ctx, kpr.State.ActionCounter, kpr.State.Actions)
	kpr.State.ActionCounter += uint64(len(kpr.State.Actions))
	kpr.State.Actions = nil
}

func (kpr *Keyper) decide() []fx.IAction {
	decider := NewDecider(kpr)
	decider.Decide()
	return decider.Actions
}

func (kpr *Keyper) runOneStep(ctx context.Context) {
	if len(kpr.State.Actions) > 0 {
		panic("internal errror: kpr.State.Actions is not empty")
	}
	kpr.State.Actions = kpr.decide()
	if err := kpr.saveState(); err != nil {
		panic(err)
	}
	kpr.runActions(ctx)
}
