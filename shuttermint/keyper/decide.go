package keyper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/ecies"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/observe"
	"shielder/shuttermint/keyper/puredkg"
	"shielder/shuttermint/medley"
	"shielder/shuttermint/shmsg"
)

type decryptfn func(encrypted []byte) ([]byte, error)

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

// DKG is used to store local state about active DKG processes. Each DKG has a corresponding
// observe.Eon struct stored in observe.Shielder, which we can find with Shielder's FindEon method.
type DKG struct {
	Eon                  uint64
	Keypers              []common.Address
	Pure                 *puredkg.PureDKG
	CommitmentsIndex     int
	PolyEvalsIndex       int
	OutgoingPolyEvalMsgs []puredkg.PolyEvalMsg
}

func (dkg *DKG) ShortInfo() string {
	return fmt.Sprintf("eon=%d, #keypers=%d, phase=%s", dkg.Eon, len(dkg.Keypers), dkg.Pure.Phase)
}

// newApology create a new shmsg apology message from the given puredkg apologies
func (dkg *DKG) newApology(apologies []puredkg.ApologyMsg) *shmsg.Message {
	var accusers []common.Address
	var polyEvals []*big.Int

	for _, a := range apologies {
		accusers = append(accusers, dkg.Keypers[a.Accuser])
		polyEvals = append(polyEvals, a.Eval)
	}
	return shmsg.NewApology(dkg.Eon, accusers, polyEvals)
}

// newAccusation creates a new shmsg accusation message from the given puredkg accusations
func (dkg *DKG) newAccusation(accusations []puredkg.AccusationMsg) *shmsg.Message {
	var accused []common.Address
	for _, a := range accusations {
		accused = append(accused, dkg.Keypers[a.Accused])
	}
	return shmsg.NewAccusation(dkg.Eon, accused)
}

func (dkg *DKG) syncCommitments(eon observe.Eon) {
	for i := dkg.CommitmentsIndex; i < len(eon.Commitments); i++ {
		comm := eon.Commitments[i]
		sender, err := medley.FindAddressIndex(dkg.Keypers, comm.Sender)
		if err != nil {
			continue
		}

		err = dkg.Pure.HandlePolyCommitmentMsg(
			puredkg.PolyCommitmentMsg{Eon: comm.Eon, Gammas: comm.Gammas, Sender: uint64(sender)},
		)
		if err != nil {
			log.Printf("Error in syncCommitments: %s", err)
		}
	}
	dkg.CommitmentsIndex = len(eon.Commitments)
}

func (dkg *DKG) syncPolyEvals(eon observe.Eon, decrypt decryptfn) {
	keyperIndex := dkg.Pure.Keyper
	for i := dkg.PolyEvalsIndex; i < len(eon.PolyEvals); i++ {
		eval := eon.PolyEvals[i]

		sender, err := medley.FindAddressIndex(dkg.Keypers, eval.Sender)
		if err != nil {
			continue
		}
		if uint64(sender) == keyperIndex {
			continue
		}

		for j, receiver := range eval.Receivers {
			receiverIndex, err := medley.FindAddressIndex(dkg.Keypers, receiver)
			if err != nil {
				log.Printf("Error in syncPolyEvals: %s", err)
				continue
			}
			if uint64(receiverIndex) != keyperIndex {
				continue
			}
			encrypted := eval.EncryptedEvals[j]
			evalBytes, err := decrypt(encrypted)
			if err != nil {
				log.Printf("Error in syncPolyEvals: %s", err)
				continue
			}
			b := new(big.Int)
			b.SetBytes(evalBytes)
			err = dkg.Pure.HandlePolyEvalMsg(
				puredkg.PolyEvalMsg{
					Eon:      eval.Eon,
					Sender:   uint64(sender),
					Receiver: keyperIndex,
					Eval:     b,
				})
			if err != nil {
				log.Printf("Error in syncPolyEvals: %s", err)
			}
		}
	}
	dkg.PolyEvalsIndex = len(eon.PolyEvals)
}

func (dkg *DKG) syncWithEon(eon observe.Eon, decrypt decryptfn) {
	dkg.syncCommitments(eon)
	dkg.syncPolyEvals(eon, decrypt)
}

// State is the keyper's internal state
type State struct {
	CheckinMessageSent       bool
	LastSentBatchConfigIndex uint64
	LastEonStarted           uint64
	DKGs                     []DKG
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
	if dcdr.State.CheckinMessageSent {
		return false
	}
	if dcdr.Shielder.IsCheckedIn(dcdr.Config.Address()) {
		return false
	}
	return dcdr.Shielder.IsKeyper(dcdr.Config.Address())
}

func (dcdr *Decider) sendCheckIn() {
	validatorPublicKey := dcdr.Config.ValidatorKey.Public().(ed25519.PublicKey)
	msg := shmsg.NewCheckIn([]byte(validatorPublicKey), &dcdr.Config.EncryptionKey.PublicKey)
	dcdr.sendShielderMessage("checkin", msg)
}

func (dcdr *Decider) maybeSendCheckIn() {
	if dcdr.shouldSendCheckin() {
		dcdr.sendCheckIn()
		dcdr.State.CheckinMessageSent = true
	}
}

func (dcdr *Decider) sendBatchConfig(configIndex uint64, config contract.BatchConfig) {
	msg := shmsg.NewBatchConfig(
		config.StartBatchIndex,
		config.Keypers,
		config.Threshold,
		dcdr.Config.ConfigContractAddress,
		configIndex,
		false,
		false,
	)
	dcdr.sendShielderMessage(fmt.Sprintf("batch config, index=%d", configIndex), msg)
}

func (dcdr *Decider) maybeSendBatchConfig() {
	if len(dcdr.Shielder.BatchConfigs) == 0 {
		log.Printf("shielder is not bootstrapped")
		return
	}
	configIndex := 1 + dcdr.Shielder.BatchConfigs[len(dcdr.Shielder.BatchConfigs)-1].ConfigIndex

	if configIndex <= dcdr.State.LastSentBatchConfigIndex {
		return // already sent this one out
	}

	if configIndex < uint64(len(dcdr.MainChain.BatchConfigs)) {
		dcdr.sendBatchConfig(configIndex, dcdr.MainChain.BatchConfigs[configIndex])
		dcdr.State.LastSentBatchConfigIndex = configIndex
	}
}

func (dcdr *Decider) sendEonStartVoting(startBatchIndex uint64) {
	msg := shmsg.NewEonStartVote(startBatchIndex)
	dcdr.sendShielderMessage(fmt.Sprintf("eon start voting, startBatchIndex=%d", startBatchIndex), msg)
}

func (dcdr *Decider) startDKG(eon observe.Eon) {
	batchConfig := dcdr.Shielder.FindBatchConfigByBatchIndex(eon.StartEvent.BatchIndex)
	keyperIndex, err := medley.FindAddressIndex(batchConfig.Keypers, dcdr.Config.Address())
	if err != nil {
		return
	}

	pure := puredkg.NewPureDKG(eon.Eon, uint64(len(batchConfig.Keypers)), batchConfig.Threshold, uint64(keyperIndex))
	dkg := DKG{Eon: eon.Eon, Pure: &pure, Keypers: batchConfig.Keypers}
	dcdr.State.DKGs = append(dcdr.State.DKGs, dkg)
}

func (dcdr *Decider) maybeStartDKG() {
	for _, eon := range dcdr.Shielder.Eons {
		if eon.Eon > dcdr.State.LastEonStarted {
			// TODO we should check that we do not start eons that are in the past
			dcdr.startDKG(eon)
			dcdr.State.LastEonStarted = eon.Eon
		}
	}
}

// PhaseLength is used to store the accumulated lengths of the DKG phases
type PhaseLength struct {
	Off         int64
	Dealing     int64
	Accusing    int64
	Apologizing int64
}

var phaseLength = PhaseLength{
	Off:         0,
	Dealing:     30,
	Accusing:    60,
	Apologizing: 90,
}

// sendPolyEvals sends the outgoing PolyEvalMsg stored in dkg that can be sent. A PolyEvalMessage
// can only be sent, when we do have the receiver's public encryption key. If we're beyond the
// 'Dealing' phase, it's too late to send these messages. In that case we clear the
// OutgoingPolyEvalMsgs field and log a message.
func (dcdr *Decider) sendPolyEvals(dkg *DKG) {
	if len(dkg.OutgoingPolyEvalMsgs) == 0 {
		return
	}

	if dkg.Pure.Phase > puredkg.Dealing {
		log.Printf(
			"Warning: could not send %d poly eval messages for eon %d, because the dealing phase is already over",
			len(dkg.OutgoingPolyEvalMsgs),
			dkg.Eon)
		dkg.OutgoingPolyEvalMsgs = nil
		return
	}

	var newOutgoing []puredkg.PolyEvalMsg
	var receivers []common.Address
	var encryptedEvals [][]byte

	for _, p := range dkg.OutgoingPolyEvalMsgs {
		receiver := dkg.Keypers[p.Receiver]
		encryptionKey, ok := dcdr.Shielder.KeyperEncryptionKeys[receiver]
		if ok {
			encrypted, err := ecies.Encrypt(rand.Reader, encryptionKey, p.Eval.Bytes(), nil, nil)
			if err != nil {
				panic(err)
			}
			encryptedEvals = append(encryptedEvals, encrypted)
			receivers = append(receivers, receiver)
		} else {
			newOutgoing = append(newOutgoing, p)
		}
	}
	if len(receivers) > 0 {
		dcdr.sendShielderMessage(
			fmt.Sprintf("poly eval, eon=%d, %d receivers, %d still outgoing", dkg.Eon, len(receivers), len(newOutgoing)),
			shmsg.NewPolyEval(dkg.Eon, receivers, encryptedEvals))
		dkg.OutgoingPolyEvalMsgs = newOutgoing
		if len(dkg.OutgoingPolyEvalMsgs) == 0 {
			log.Printf("Sent all poly eval messages for eon %d", dkg.Eon)
		}
	}
}

func (dcdr *Decider) startPhase1Dealing(dkg *DKG) {
	commitment, polyEvals, err := dkg.Pure.StartPhase1Dealing()
	if err != nil {
		panic(err) // XXX fix error handling
	}

	dkg.OutgoingPolyEvalMsgs = polyEvals

	dcdr.sendShielderMessage(
		fmt.Sprintf("poly commitment, eon=%d", dkg.Eon),
		shmsg.NewPolyCommitment(dkg.Eon, commitment.Gammas))
}

func (dcdr *Decider) startPhase2Accusing(dkg *DKG) {
	accusations := dkg.Pure.StartPhase2Accusing()
	dcdr.sendShielderMessage(
		fmt.Sprintf("accusations, eon=%d, count=%d", dkg.Eon, len(accusations)),
		dkg.newAccusation(accusations))
}

func (dcdr *Decider) startPhase3Apologizing(dkg *DKG) {
	apologies := dkg.Pure.StartPhase3Apologizing()
	dcdr.sendShielderMessage(
		fmt.Sprintf("apologies, eon=%d, count=%d", dkg.Eon, len(apologies)),
		dkg.newApology(apologies))
}

func (dcdr *Decider) dkgFinalize(dkg *DKG) {
	dkg.Pure.Finalize()
}

func (dcdr *Decider) dkgStartNextPhase(dkg *DKG, eon *observe.Eon) {
	if dcdr.Shielder.CurrentBlock >= eon.StartHeight+phaseLength.Off &&
		dkg.Pure.Phase <= puredkg.Off {
		dcdr.startPhase1Dealing(dkg)
	}
	if dcdr.Shielder.CurrentBlock >= eon.StartHeight+phaseLength.Dealing &&
		dkg.Pure.Phase <= puredkg.Dealing {
		dcdr.startPhase2Accusing(dkg)
	}

	if dcdr.Shielder.CurrentBlock >= eon.StartHeight+phaseLength.Accusing &&
		dkg.Pure.Phase <= puredkg.Accusing {
		dcdr.startPhase3Apologizing(dkg)
	}

	if dcdr.Shielder.CurrentBlock >= eon.StartHeight+phaseLength.Apologizing &&
		dkg.Pure.Phase <= puredkg.Apologizing {
		dcdr.dkgFinalize(dkg)
	}
}

func (dcdr *Decider) handleDKGs() {
	decrypt := func(encrypted []byte) ([]byte, error) {
		return dcdr.Config.EncryptionKey.Decrypt(encrypted, []byte(""), []byte(""))
	}
	for i := range dcdr.State.DKGs {
		dkg := &dcdr.State.DKGs[i]
		eon, err := dcdr.Shielder.FindEon(dkg.Eon)
		if err != nil {
			panic(err)
		}
		dkg.syncWithEon(*eon, decrypt)
		dcdr.dkgStartNextPhase(dkg, eon)
		dcdr.sendPolyEvals(dkg)
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
	dcdr.handleDKGs()
}
