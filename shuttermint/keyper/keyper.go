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
	"sync/atomic"
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
	// Keep the atomic.Value as first field in order to make sure it's 64-bit aligned. Visit
	// https://golang.org/pkg/sync/atomic/#pkg-note-BUG for more information
	world atomic.Value // holds an observe.World struct

	Config KeyperConfig
	State  *State

	ContractCaller contract.Caller
	shmcl          client.Client
	MessageSender  fx.MessageSender
	lastlogTime    time.Time
	runenv         *fx.RunEnv
}

func NewKeyper(kc KeyperConfig) Keyper {
	world := atomic.Value{}
	world.Store(observe.World{
		Shielder:   observe.NewShielder(),
		MainChain: observe.NewMainChain(kc.MainChainFollowDistance),
	})

	return Keyper{
		Config: kc,
		State:  NewState(),
		world:  world,
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

func (kpr *Keyper) dkginfo() string {
	var ds []string
	for i := len(kpr.State.DKGs) - 1; i >= 0; i-- {
		dkg := kpr.State.DKGs[i]
		if dkg.IsFinalized() {
			break
		}
		ds = append(ds, dkg.ShortInfo())
	}

	if len(ds) == 0 {
		return ""
	}
	return fmt.Sprintf(", DKGs: %s", strings.Join(ds, " - "))
}

func (kpr *Keyper) ShortInfo() string {
	world := kpr.CurrentWorld()
	return fmt.Sprintf(
		"shielder block %d, main chain %d, %s, last eon started %d, num half steps: %d%s",
		world.Shielder.CurrentBlock,
		world.MainChain.CurrentBlock,
		kpr.runenv.ShortInfo(),
		kpr.State.LastEonStarted,
		world.MainChain.NumExecutionHalfSteps,
		kpr.dkginfo(),
	)
}

func (kpr *Keyper) dumpInternalState() {
	log.Printf("Received signal. Dumping internal state")
	world := kpr.CurrentWorld()
	pretty.Println("Shielder:", world.Shielder)
	pretty.Println("Mainchain:", world.MainChain)
	pretty.Println("State:", kpr.State)
}

func (kpr *Keyper) Run() error {
	err := kpr.init()
	if err != nil {
		return err
	}
	g, ctx := errgroup.WithContext(context.Background())

	mainChains := make(chan *observe.MainChain)
	shielders := make(chan *observe.Shielder)
	signals := make(chan os.Signal, 1)
	filter := make(chan observe.ShielderFilter, 3)
	signal.Notify(signals, syscall.SIGUSR1)
	g.Go(func() error {
		return observe.SyncMain(ctx, &kpr.ContractCaller, kpr.CurrentWorld().MainChain, mainChains)
	})
	g.Go(func() error {
		return observe.SyncShielder(ctx, kpr.shmcl, kpr.CurrentWorld().Shielder, shielders, filter)
	})

	var world observe.World
	// Sync main and shielder chain at least once. Otherwise, the state of one of the two will
	// be much more recent than the other one when the first block appears.
	for world.MainChain == nil || world.Shielder == nil {
		select {
		case <-signals:
			kpr.dumpInternalState()
			continue
		case mainChain := <-mainChains:
			world.MainChain = mainChain
		case shielder := <-shielders:
			world.Shielder = shielder
		case <-ctx.Done():
			return g.Wait()
		}
	}
	kpr.world.Store(world)
	kpr.runenv.StartBackgroundTasks(ctx, g)
	havePendingActions, err := kpr.runenv.Load()
	if err != nil {
		return err
	}

	if !havePendingActions && len(kpr.State.Actions) == 0 && kpr.State.PendingHalfStep != nil {
		log.Printf("Fixing State: PendingHalfStep should be nil!")
		kpr.State.PendingHalfStep = nil
	}

	if len(kpr.State.Actions) > 0 {
		kpr.runActions(ctx)
	}

	for {
		select {
		case <-signals:
			kpr.dumpInternalState()
			continue
		case <-ctx.Done():
			return g.Wait()
		case mainChain := <-mainChains:
			world.MainChain = mainChain
		case shielder := <-shielders:
			world.Shielder = shielder
		}
		kpr.world.Store(world)
		kpr.runOneStep(ctx)
		filter <- kpr.State.GetShielderFilter()
	}
}

func (kpr *Keyper) CurrentWorld() observe.World {
	return kpr.world.Load().(observe.World)
}

type storedState struct {
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain
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

	if st.State.SyncHeight == 0 && st.Shielder.CurrentBlock > 0 {
		log.Printf("Fixing SyncHeight: %d", st.Shielder.CurrentBlock)
		st.State.SyncHeight = st.Shielder.CurrentBlock // We didn't have this field in older versions
	}
	kpr.State = st.State
	world := observe.World{
		Shielder:   st.Shielder,
		MainChain: st.MainChain,
	}
	kpr.world.Store(world)

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
	world := kpr.CurrentWorld()
	st := storedState{
		State:     kpr.State,
		Shielder:   world.Shielder,
		MainChain: world.MainChain,
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
