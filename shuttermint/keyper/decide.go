package keyper

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/observe"
	"shielder/shuttermint/keyper/puredkg"
	"shielder/shuttermint/medley"
	"shielder/shuttermint/shmsg"
)

// IRunEnv is passed as a parameter to IAction's Run function. At the moment this only allows
// interaction with the shielder chain. We will also need a way to talk to the main chain
type IRunEnv interface {
	MessageSender
}

// IAction describes an action to run as determined by the Decider's Decide method.
type IAction interface {
	Run(ctx context.Context, runenv IRunEnv) error
}

var (
	_ IAction = FakeAction{}
	_ IAction = SendShielderMessage{}
)

type DKG struct {
	Eon  uint64
	Pure *puredkg.PureDKG
}

// State is the keyper's internal state
type State struct {
	checkinMessageSent       bool
	lastSentBatchConfigIndex uint64
	lastEonStarted           uint64
	dkgs                     []DKG
}

// Decider decides on the next actions to take based on our internal State and the current Shielder
// and MainChain state for a single step. For each step the keyper creates a new Decider. The
// actions to run are stored inside the Actions field.
type Decider struct {
	Config    KeyperConfig
	State     *State
	Shielder   *observe.Shielder
	MainChain *observe.MainChain
	Actions   []IAction
}

// FakeAction only prints a message to the log. It's useful only during development as a
// placeholder for the real action.  XXX needs to be removed!
type FakeAction struct {
	msg string
}

func (a FakeAction) Run(_ context.Context, _ IRunEnv) error {
	log.Printf("Run: %s", a.msg)
	return nil
}

// SendShielderMessage is a Action that send's a message to shuttermint
type SendShielderMessage struct {
	description string
	msg         *shmsg.Message
}

func (a SendShielderMessage) Run(ctx context.Context, runenv IRunEnv) error {
	log.Printf("Run: %s", a)
	return runenv.SendMessage(ctx, a.msg)
}

func (a SendShielderMessage) String() string {
	return fmt.Sprintf("-> shuttermint: %s", a.description)
}

// addAction stores the given IAction to be run later
func (dcdr *Decider) addAction(a IAction) {
	dcdr.Actions = append(dcdr.Actions, a)
}

func (dcdr *Decider) sendShielderMessage(description string, msg *shmsg.Message) {
	dcdr.addAction(SendShielderMessage{
		description: description,
		msg:         msg,
	})
}

// shouldSendCheckin returns true if we should send the CheckIn message
func (dcdr *Decider) shouldSendCheckin() bool {
	if dcdr.State.checkinMessageSent {
		return false
	}
	if dcdr.Shielder.IsCheckedIn(dcdr.Config.Address()) {
		return false
	}
	return dcdr.Shielder.IsKeyper(dcdr.Config.Address())
}

func (dcdr *Decider) sendCheckIn() {
	validatorPublicKey := dcdr.Config.ValidatorKey.Public().(ed25519.PublicKey)
	msg := NewCheckIn([]byte(validatorPublicKey), &dcdr.Config.EncryptionKey.PublicKey)
	dcdr.sendShielderMessage("checkin", msg)
}

func (dcdr *Decider) maybeSendCheckIn() {
	if dcdr.shouldSendCheckin() {
		dcdr.sendCheckIn()
		dcdr.State.checkinMessageSent = true
	}
}

func (dcdr *Decider) sendBatchConfig(configIndex uint64, config contract.BatchConfig) {
	msg := NewBatchConfig(
		config.StartBatchIndex,
		config.Keypers,
		config.Threshold,
		dcdr.Config.ConfigContractAddress,
		configIndex,
	)
	dcdr.sendShielderMessage(fmt.Sprintf("batch config, index=%d", configIndex), msg)
}

func (dcdr *Decider) maybeSendBatchConfig() {
	if len(dcdr.Shielder.BatchConfigs) == 0 {
		log.Printf("shielder is not bootstrapped")
		return
	}
	configIndex := 1 + dcdr.Shielder.BatchConfigs[len(dcdr.Shielder.BatchConfigs)-1].ConfigIndex

	if configIndex <= dcdr.State.lastSentBatchConfigIndex {
		return // already sent this one out
	}

	if configIndex < uint64(len(dcdr.MainChain.BatchConfigs)) {
		dcdr.sendBatchConfig(configIndex, dcdr.MainChain.BatchConfigs[configIndex])
		dcdr.State.lastSentBatchConfigIndex = configIndex
	}
}

func (dcdr *Decider) sendEonStartVoting(startBatchIndex uint64) {
	msg := NewEonStartVoteMsg(startBatchIndex)
	dcdr.sendShielderMessage(fmt.Sprintf("eon start voting, startBatchIndex=%d", startBatchIndex), msg)
}

func (dcdr *Decider) startDKG(eon observe.Eon) {
	batchConfig := dcdr.Shielder.FindBatchConfigByBatchIndex(eon.StartEvent.BatchIndex)
	keyperIndex, err := medley.FindAddressIndex(batchConfig.Keypers, dcdr.Config.Address())
	if err != nil {
		return
	}

	pure := puredkg.NewPureDKG(eon.Eon, uint64(len(batchConfig.Keypers)), batchConfig.Threshold, uint64(keyperIndex))
	commitment, evals, err := pure.StartPhase1Dealing()
	if err != nil {
		return
	}
	_ = evals // XXX we need to send them as well
	msg := NewPolyCommitmentMsg(eon.Eon, commitment.Gammas)
	dcdr.sendShielderMessage(fmt.Sprintf("poly commitment, eon=%d", eon.Eon), msg)
	dkg := DKG{Eon: eon.Eon, Pure: &pure}
	dcdr.State.dkgs = append(dcdr.State.dkgs, dkg)
}

func (dcdr *Decider) maybeStartDKG() {
	for _, eon := range dcdr.Shielder.Eons {
		if eon.Eon > dcdr.State.lastEonStarted {
			// TODO we should check that we do not start eons that are in the past
			dcdr.startDKG(eon)
			dcdr.State.lastEonStarted = eon.Eon
		}
	}
}

// Decide determines the next actions to run.
func (dcdr *Decider) Decide() {
	// We can't go on unless we're registered as keyper in shuttermint
	if !dcdr.Shielder.IsKeyper(dcdr.Config.Address()) {
		log.Printf("not registered as keyper in shuttermint, nothing to do")
		return
	}
	dcdr.maybeSendCheckIn()
	dcdr.maybeSendBatchConfig()
	dcdr.maybeStartDKG()
}
