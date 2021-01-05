package keyper

import (
	"bufio"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/kr/pretty"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	"golang.org/x/sync/errgroup"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/observe"
)

type Keyper2 struct {
	Config    KeyperConfig
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain

	ContractCaller ContractCaller
	shmcl          client.Client
	MessageSender  MessageSender
	Interactive    bool
}

func NewKeyper2(kc KeyperConfig) Keyper2 {
	return Keyper2{
		Config:    kc,
		State:     &State{},
		Shielder:   observe.NewShielder(),
		MainChain: observe.NewMainChain(),
	}
}

func NewContractCallerFromConfig(config KeyperConfig) (ContractCaller, error) {
	ethcl, err := ethclient.Dial(config.EthereumURL)
	if err != nil {
		return ContractCaller{}, err
	}
	configContract, err := contract.NewConfigContract(config.ConfigContractAddress, ethcl)
	if err != nil {
		return ContractCaller{}, err
	}

	keyBroadcastContract, err := contract.NewKeyBroadcastContract(config.KeyBroadcastContractAddress, ethcl)
	if err != nil {
		return ContractCaller{}, err
	}

	batcherContract, err := contract.NewBatcherContract(config.BatcherContractAddress, ethcl)
	if err != nil {
		return ContractCaller{}, err
	}

	executorContract, err := contract.NewExecutorContract(config.ExecutorContractAddress, ethcl)
	if err != nil {
		return ContractCaller{}, err
	}

	return NewContractCaller(
		ethcl,
		config.SigningKey,
		configContract,
		keyBroadcastContract,
		batcherContract,
		executorContract,
	), nil
}

func (kpr *Keyper2) init() error {
	if kpr.shmcl != nil {
		panic("internal error: already initialized")
	}
	var err error
	kpr.shmcl, err = http.New(kpr.Config.ShielderURL, "/websocket")
	if err != nil {
		return errors.Wrapf(err, "create shuttermint client at %s", kpr.Config.ShielderURL)
	}
	ms := NewRPCMessageSender(kpr.shmcl, kpr.Config.SigningKey)
	kpr.MessageSender = &ms

	kpr.ContractCaller, err = NewContractCallerFromConfig(kpr.Config)
	return err

	// executor := Executor{
	//	ctx:                   kpr.ctx,
	//	client:                kpr.ethcl,
	//	cc:                    &contractCaller,
	//	cipherExecutionParams: kpr.cipherExecutionParams,
	// }
	// kpr.executor = executor
}

func (kpr *Keyper2) syncMain(ctx context.Context) error {
	return kpr.MainChain.SyncToHead(ctx, kpr.ContractCaller.Ethclient, kpr.ContractCaller.ConfigContract)
}

func (kpr *Keyper2) syncShielder(ctx context.Context) error {
	return kpr.Shielder.SyncToHead(ctx, kpr.shmcl)
}

func (kpr *Keyper2) sync(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return kpr.syncShielder(ctx)
	})
	group.Go(func() error {
		return kpr.syncMain(ctx)
	})
	err := group.Wait()
	return err
}

func (kpr *Keyper2) Run() error {
	err := kpr.init()
	if err != nil {
		return err
	}
	ctx := context.Background()

	for {
		err = kpr.sync(ctx)
		if err != nil {
			return err
		}

		fmt.Println("-----------------------------------------------------------------------")
		pretty.Println("SHUTTER   ==>", *kpr.Shielder)
		pretty.Println("MAINCHAIN ==>", *kpr.MainChain)
		kpr.runOneStep(ctx)
		pretty.Println("INTERNAL  ==>", *kpr.State)

		time.Sleep(10 * time.Second)
	}
}

func readline() {
	fmt.Printf("\n[press return to continue] > ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	if err := scanner.Err(); err != nil {
		log.Println(err)
	}
	fmt.Printf("\n")
}

type storedState struct {
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain
}

func (kpr *Keyper2) gobpath() string {
	return filepath.Join(kpr.Config.DBDir, "state.gob")
}

func (kpr *Keyper2) LoadState() error {
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

func (kpr *Keyper2) saveState() error {
	gobpath := kpr.gobpath()
	log.Printf("Saving state to %s", gobpath)
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

func (kpr *Keyper2) runOneStep(ctx context.Context) {
	decider := Decider{
		Config:    kpr.Config,
		State:     kpr.State,
		Shielder:   kpr.Shielder,
		MainChain: kpr.MainChain,
	}
	decider.Decide()
	if kpr.Interactive && len(decider.Actions) > 0 {
		log.Printf("Showing %d actions", len(decider.Actions))
		for _, act := range decider.Actions {
			fmt.Println(act)
		}
		readline()
	}
	err := kpr.saveState()
	if err != nil {
		panic(err)
	}
	log.Printf("Running %d actions", len(decider.Actions))

	for _, act := range decider.Actions {
		err := act.Run(ctx, kpr.MessageSender)
		// XXX at the moment we just let the whole program die. We need a better strategy
		// here. We could retry the actions or feed the errors back into our state
		if err != nil {
			panic(err)
		}
	}
}
