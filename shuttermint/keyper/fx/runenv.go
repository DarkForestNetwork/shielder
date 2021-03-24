package fx

import (
	"context"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/sync/errgroup"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/observe"
	"shielder/shuttermint/medley"
)

const (
	numMainChainWorkers = 20
)

type ActionWithID struct {
	Action IAction
	ID     ActionID
}

type RunEnv struct {
	PendingActions       *PendingActions
	PendingActionsPath   string
	MessageSender        MessageSender
	ContractCaller       *contract.Caller
	shuttermintMessages  chan ActionID
	mainChainTXs         chan ActionID
	inFlightMainChainTXs chan ActionID
	currentWorld         func() observe.World
}

func NewRunEnv(messageSender MessageSender, contractCaller *contract.Caller, currentWorld func() observe.World, path string) *RunEnv {
	return &RunEnv{
		PendingActions:       NewPendingActions(path),
		MessageSender:        messageSender,
		ContractCaller:       contractCaller,
		shuttermintMessages:  make(chan ActionID),
		mainChainTXs:         make(chan ActionID, numMainChainWorkers),
		inFlightMainChainTXs: make(chan ActionID),
		currentWorld:         currentWorld,
	}
}

func (runenv *RunEnv) sendShielderMessage(ctx context.Context, id ActionID, act *SendShielderMessage) error {
	log.Printf("=====%s, id=%d", act, id)
	err := runenv.MessageSender.SendMessage(ctx, act.Msg)
	return err
}

func (runenv *RunEnv) sendMainChainTX(ctx context.Context, id ActionID, act MainChainTX) error {
	var err error
	var tx *types.Transaction
	var auth *bind.TransactOpts

	auth, err = runenv.ContractCaller.Auth()
	auth.Context = ctx

	if err != nil {
		return err
	}
	tx, err = act.SendTX(runenv.ContractCaller, auth)
	if err != nil {
		return err
	}
	runenv.PendingActions.SetMainChainTXHash(id, tx.Hash())
	runenv.inFlightMainChainTXs <- id
	return nil
}

var zerohash = common.Hash{}

func (runenv *RunEnv) waitMined(ctx context.Context, id ActionID) {
	act := runenv.PendingActions.GetAction(id)
	hash := runenv.PendingActions.GetMainChainTXHash(id)
	if hash == zerohash {
		log.Fatalf("internal error: cannot wait for the zero hash, id=%d", id)
	}
	receipt, err := medley.WaitMined(ctx, runenv.ContractCaller.Ethclient, hash)
	if err != nil {
		log.Printf("Error waiting for transaction id=%d, %s: %v", id, hash.Hex(), err)
		return
	}
	if receipt == nil {
		// This happens if the context is canceled.
		return
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		world := runenv.CurrentWorld() // XXX we should make sure our world includes the receipt's blocknumber
		expired := act.IsExpired(world)
		log.Printf("TX reverted: id=%d, expired=%t, %s, hash=%s", id, expired, act, hash.Hex())
	} else {
		log.Printf("TX success: id=%d, %s, hash=%s", id, act, hash.Hex())
	}
}

func (runenv *RunEnv) RunActions(ctx context.Context, actionCounter uint64, actions []IAction) {
	if len(actions) == 0 {
		return
	}

	log.Printf("Running %d actions", len(actions))
	startID, endID := runenv.PendingActions.AddActions(ActionID(actionCounter), actions)
	for id := startID; id < endID; id++ {
		runenv.scheduleAction(id)
	}
}

// scheduleAction schedules an action to be run. The given action must already be stored in the
// pending actions struct.
func (runenv *RunEnv) scheduleAction(id ActionID) {
	act := runenv.PendingActions.GetAction(id)
	switch a := act.(type) {
	case *SendShielderMessage:
		runenv.shuttermintMessages <- id
	case MainChainTX:
		txhash := runenv.PendingActions.GetMainChainTXHash(id)
		if txhash != zerohash {
			runenv.inFlightMainChainTXs <- id
		} else {
			runenv.mainChainTXs <- id
		}
	default:
		log.Fatalf("cannot run %s", a)
	}
}

// Load loads the pending actions from disk and schedules the actions to be run.
func (runenv *RunEnv) Load() error {
	err := runenv.PendingActions.Load()
	if err != nil {
		return err
	}

	for _, id := range runenv.PendingActions.SortedIDs() {
		runenv.scheduleAction(id)
	}
	return nil
}

func (runenv *RunEnv) handleAction(ctx context.Context, id ActionID, action IAction) (bool, error) {
	switch a := action.(type) {
	case *SendShielderMessage:
		err := runenv.sendShielderMessage(ctx, id, a)
		if err != nil {
			return false, err
		}
		return true, nil
	case MainChainTX:
		err := runenv.sendMainChainTX(ctx, id, a)
		if err != nil {
			return false, err
		}
		return false, nil
	default:
		log.Fatalf("internal error: handleAction: cannot handle %s", action)
		return false, nil
	}
}

func (runenv *RunEnv) handleActions(ctx context.Context, actions chan ActionID) {
	for {
		select {
		case id := <-actions:
			a := runenv.PendingActions.GetAction(id)
			var err error
			var remove bool

			for {
				if a.IsExpired(runenv.CurrentWorld()) {
					log.Printf("Action expired: id=%d, %s", id, a)
					remove = true
					break
				}
				if err != nil {
					log.Printf("Retrying action id=%d, %s; err=%s", id, a, err)
				}
				remove, err = runenv.handleAction(ctx, id, a)
				if err == nil {
					break
				}
				if !IsRetriable(err) {
					remove = true
					log.Printf("Non-retriable error id=%d, %s; err=%s", id, a, err)
					break
				}
				time.Sleep(time.Second)
			}
			if remove {
				runenv.PendingActions.RemoveAction(id)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (runenv *RunEnv) handleInFlightTXs(ctx context.Context) {
	for {
		select {
		case id := <-runenv.inFlightMainChainTXs:
			runenv.waitMined(ctx, id)
			runenv.PendingActions.RemoveAction(id)
		case <-ctx.Done():
			return
		}
	}
}

func (runenv *RunEnv) CurrentWorld() observe.World {
	return runenv.currentWorld()
}

func (runenv *RunEnv) StartBackgroundTasks(ctx context.Context, g *errgroup.Group) {
	// Start exactly one worker, because we like to make sure the actions are started in the
	// order given to us
	for _, ch := range []chan ActionID{runenv.mainChainTXs, runenv.shuttermintMessages} {
		ch := ch
		g.Go(func() error {
			runenv.handleActions(ctx, ch)
			return nil
		})
	}

	for i := 0; i < numMainChainWorkers; i++ {
		g.Go(func() error {
			runenv.handleInFlightTXs(ctx)
			return nil
		})
	}
}
