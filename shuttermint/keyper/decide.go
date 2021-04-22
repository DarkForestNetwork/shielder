package keyper

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	pkgErrors "github.com/pkg/errors"
	"golang.org/x/crypto/sha3"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper/epochkg"
	"shielder/shuttermint/keyper/fx"
	"shielder/shuttermint/keyper/observe"
	"shielder/shuttermint/keyper/puredkg"
	"shielder/shuttermint/medley"
	"shielder/shuttermint/shcrypto"
	"shielder/shuttermint/shmsg"
)

// maxParallelHalfSteps is the maximum number of txs to send at the same time to execute half
// steps.
const maxParallelHalfSteps uint64 = 10

type decryptfn func(encrypted []byte) ([]byte, error)

// Batch is used to store local state about a single Batch.
type Batch struct {
	BatchIndex               uint64
	DecryptionSignatureHash  []byte
	DecryptedTransactions    [][]byte
	DecryptedBatchHash       []byte
	DecryptionSignatureIndex int
	SignatureCount           int // number of valid signatures
}

// VerifySignature checks if the sender signed the batches' DecryptionSignatureHash.
func (batch *Batch) VerifySignature(sender common.Address, signature []byte) bool {
	pubkey, err := crypto.SigToPub(batch.DecryptionSignatureHash, signature)
	if err != nil {
		return false
	}
	signer := crypto.PubkeyToAddress(*pubkey)
	return signer == sender
}

// DKG is used to store local state about active DKG processes. Each DKG has a corresponding
// observe.Eon struct stored in observe.Shielder, which we can find with Shielder's FindEon method.
type DKG struct {
	Eon                  uint64
	StartBatchIndex      uint64
	Keypers              []common.Address
	Pure                 *puredkg.PureDKG
	OutgoingPolyEvalMsgs []puredkg.PolyEvalMsg
	PhaseLength          PhaseLength
}

// EKG is used to store local state about the epoch key generation process.
type EKG struct {
	Eon     uint64
	Keypers []common.Address
	EpochKG *epochkg.EpochKG
}

func (dkg *DKG) ShortInfo() string {
	return fmt.Sprintf("eon=%d, #keypers=%d, %s", dkg.Eon, len(dkg.Keypers), dkg.Pure.ShortInfo())
}

func (dkg *DKG) IsFinalized() bool {
	return dkg.Pure == nil || dkg.Pure.Phase == puredkg.Finalized
}

// newApology create a new shmsg apology message from the given puredkg apologies.
func (dkg *DKG) newApology(apologies []puredkg.ApologyMsg) *shmsg.Message {
	var accusers []common.Address
	var polyEvals []*big.Int

	for _, a := range apologies {
		accusers = append(accusers, dkg.Keypers[a.Accuser])
		polyEvals = append(polyEvals, a.Eval)
	}
	return shmsg.NewApology(dkg.Eon, accusers, polyEvals)
}

// newAccusation creates a new shmsg accusation message from the given puredkg accusations.
func (dkg *DKG) newAccusation(accusations []puredkg.AccusationMsg) *shmsg.Message {
	var accused []common.Address
	for _, a := range accusations {
		accused = append(accused, dkg.Keypers[a.Accused])
	}
	return shmsg.NewAccusation(dkg.Eon, accused)
}

func (dkg *DKG) syncCommitments(syncHeight int64, eon observe.Eon) {
	for _, comm := range eon.GetPolyCommitments(syncHeight) {
		phase := dkg.PhaseLength.getPhaseAtHeight(comm.Height, eon.StartHeight)
		if phase != puredkg.Dealing {
			log.Printf("Warning: received commitment in wrong phase %s: %+v", phase, comm)
			continue
		}

		sender, err := medley.FindAddressIndex(dkg.Keypers, comm.Sender)
		if err != nil {
			continue
		}

		err = dkg.Pure.HandlePolyCommitmentMsg(
			puredkg.PolyCommitmentMsg{Eon: comm.Eon, Gammas: comm.Gammas, Sender: uint64(sender)},
		)
		if err != nil {
			log.Printf("Error in syncCommitments: %+v", err)
		}
	}
}

func (dkg *DKG) syncPolyEvals(syncHeight int64, eon observe.Eon, decrypt decryptfn) {
	keyperIndex := dkg.Pure.Keyper
	for _, eval := range eon.GetPolyEvals(syncHeight) {
		phase := dkg.PhaseLength.getPhaseAtHeight(eval.Height, eon.StartHeight)
		if phase != puredkg.Dealing {
			log.Printf("Warning: received polyeval in wrong phase %s: %+v", phase, eval)
			continue
		}

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
				log.Printf("Error in syncPolyEvals: %+v", err)
				continue
			}
			if uint64(receiverIndex) != keyperIndex {
				continue
			}
			encrypted := eval.EncryptedEvals[j]
			evalBytes, err := decrypt(encrypted)
			if err != nil {
				log.Printf("Error in syncPolyEvals: %+v", err)
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
				log.Printf("Error in syncPolyEvals: %+v", err)
			}
		}
	}
}

func (dkg *DKG) syncAccusations(syncHeight int64, eon observe.Eon) {
	for _, accusation := range eon.GetAccusations(syncHeight) {
		phase := dkg.PhaseLength.getPhaseAtHeight(accusation.Height, eon.StartHeight)
		if phase != puredkg.Accusing {
			log.Printf("Warning: received accusation in wrong phase %s: %+v", phase, accusation)
			continue
		}

		sender, err := medley.FindAddressIndex(dkg.Keypers, accusation.Sender)
		if err != nil {
			log.Printf("Error: cannot handle accusation. bad sender: %s", accusation.Sender.Hex())
			continue
		}
		for _, accused := range accusation.Accused {
			accusedIndex, err := medley.FindAddressIndex(dkg.Keypers, accused)
			if err != nil {
				log.Printf("Error in syncAccusations: %+v", err)
				continue
			}
			err = dkg.Pure.HandleAccusationMsg(
				puredkg.AccusationMsg{
					Eon:     dkg.Eon,
					Accuser: uint64(sender),
					Accused: uint64(accusedIndex),
				})
			if err != nil {
				log.Printf("Error: cannot handle accusation: %+v", err)
			}
		}
	}
}

func (dkg *DKG) syncApologies(syncHeight int64, eon observe.Eon) {
	for _, apology := range eon.GetApologies(syncHeight) {
		phase := dkg.PhaseLength.getPhaseAtHeight(apology.Height, eon.StartHeight)
		if phase != puredkg.Apologizing {
			log.Printf("Warning: received apology in wrong phase %s: %+v", phase, apology)
			continue
		}

		sender, err := medley.FindAddressIndex(dkg.Keypers, apology.Sender)
		if err != nil {
			log.Printf("Error: cannot handle apology. bad sender: %s", apology.Sender.Hex())
			continue
		}
		for j, accuser := range apology.Accusers {
			accuserIndex, err := medley.FindAddressIndex(dkg.Keypers, accuser)
			if err != nil {
				log.Printf("Error in syncApologies: %+v", err)
				continue
			}
			err = dkg.Pure.HandleApologyMsg(
				puredkg.ApologyMsg{
					Eon:     dkg.Eon,
					Accuser: uint64(accuserIndex),
					Accused: uint64(sender),
					Eval:    apology.PolyEval[j],
				})
			if err != nil {
				log.Printf("Error: cannot handle apology: %+v", err)
			}
		}
	}
}

// State is the keyper's internal state.
type State struct {
	CheckInMessageSent       bool
	LastSentBatchConfigIndex uint64
	LastEonStarted           uint64
	DKGs                     []DKG
	EKGs                     []*EKG
	PendingHalfStep          *uint64
	PendingAppeals           map[uint64]struct{}
	NextEpochSecretShare     uint64
	Batches                  map[uint64]*Batch
	HalfStepsChecked         uint64

	// We store the actions that should be executed together with a counter. When starting the
	// program, we feed these actions into runenv, which can use the counter to identify the
	// actions.
	ActionCounter uint64
	Actions       []fx.IAction

	SyncHeight int64
}

// NewState creates an empty State object.
func NewState() *State {
	return &State{
		PendingAppeals: make(map[uint64]struct{}),
		Batches:        make(map[uint64]*Batch),
	}
}

// GetShielderFilter returns the shielder filter to be applied to the Shielder state.
func (st *State) GetShielderFilter(mainChain *observe.MainChain) observe.ShielderFilter {
	return observe.ShielderFilter{
		SyncHeight: st.SyncHeight,
		BatchIndex: mainChain.NumExecutionHalfSteps / 2,
	}
}

// Decider decides on the next actions to take based on our internal State and the current Shielder
// and MainChain state for a single step. For each step the keyper creates a new Decider. The
// actions to run are stored inside the Actions field.
type Decider struct {
	Config      Config
	State       *State
	Shielder     *observe.Shielder
	MainChain   *observe.MainChain
	Actions     []fx.IAction
	PhaseLength PhaseLength
}

func NewDecider(kpr *Keyper) Decider {
	world := kpr.CurrentWorld()
	return Decider{
		Config:      kpr.Config,
		State:       kpr.State,
		Shielder:     world.Shielder,
		MainChain:   world.MainChain,
		Actions:     []fx.IAction{},
		PhaseLength: NewConstantPhaseLength(int64(kpr.Config.DKGPhaseLength)),
	}
}

var errEKGNotFound = errors.New("EKG not found")

func (st *State) FindEKGByEon(eon uint64) (*EKG, error) {
	for _, epochkg := range st.EKGs {
		if epochkg.Eon == eon {
			return epochkg, nil
		}
	}
	return nil, pkgErrors.WithStack(errEKGNotFound)
}

// addAction stores the given IAction to be run later.
func (dcdr *Decider) addAction(a fx.IAction) {
	if reflect.ValueOf(a).Kind() != reflect.Ptr {
		panic("internal error: addAction: expected pointer")
	}
	dcdr.Actions = append(dcdr.Actions, a)
}

func (dcdr *Decider) sendShielderMessage(description string, msg *shmsg.Message) {
	dcdr.addAction(&fx.SendShielderMessage{
		Description: description,
		Msg:         msg,
	})
}

// shouldSendCheckIn returns true if we should send the CheckIn message.
func (dcdr *Decider) shouldSendCheckIn() bool {
	if dcdr.State.CheckInMessageSent {
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
	dcdr.sendShielderMessage("check-in", msg)
}

func (dcdr *Decider) maybeSendCheckIn() {
	if dcdr.shouldSendCheckIn() {
		dcdr.sendCheckIn()
		dcdr.State.CheckInMessageSent = true
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

func (dcdr *Decider) startDKG(eon observe.Eon) {
	batchConfig := dcdr.Shielder.FindBatchConfigByBatchIndex(eon.StartEvent.BatchIndex)
	keyperIndex, err := medley.FindAddressIndex(batchConfig.Keypers, dcdr.Config.Address())
	if err != nil {
		return
	}

	pure := puredkg.NewPureDKG(eon.Eon, uint64(len(batchConfig.Keypers)), batchConfig.Threshold, uint64(keyperIndex))
	dkg := DKG{
		Eon:             eon.Eon,
		StartBatchIndex: eon.StartEvent.BatchIndex,
		Pure:            &pure,
		Keypers:         batchConfig.Keypers,
		PhaseLength:     dcdr.PhaseLength,
	}
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

// PhaseLength is used to store the accumulated lengths of the DKG phases.
type PhaseLength struct {
	Off         int64
	Dealing     int64
	Accusing    int64
	Apologizing int64
}

// NewConstantPhaseLength creates a new phase length definition where each phase has the same
// length.
func NewConstantPhaseLength(l int64) PhaseLength {
	return PhaseLength{
		Off:         0 * l,
		Dealing:     1 * l,
		Accusing:    2 * l,
		Apologizing: 3 * l,
	}
}

func (plen *PhaseLength) getPhaseAtHeight(height int64, eonStartHeight int64) puredkg.Phase {
	if height < eonStartHeight+plen.Off {
		return puredkg.Off
	}
	if height < eonStartHeight+plen.Dealing {
		return puredkg.Dealing
	}
	if height < eonStartHeight+plen.Accusing {
		return puredkg.Accusing
	}
	if height < eonStartHeight+plen.Apologizing {
		return puredkg.Apologizing
	}
	return puredkg.Finalized
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
			encrypted, err := encryptionKey.Encrypt(rand.Reader, p.Eval.Bytes())
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

func (dcdr *Decider) startPhase1Dealing(dkg *DKG, phaseAtNextBlockHeight puredkg.Phase) {
	commitment, polyEvals, err := dkg.Pure.StartPhase1Dealing()
	if err != nil {
		log.Fatalf("Aborting due to unexpected error: %+v", err)
	}
	if phaseAtNextBlockHeight != puredkg.Dealing {
		return
	}

	dkg.OutgoingPolyEvalMsgs = polyEvals
	dcdr.sendShielderMessage(
		fmt.Sprintf("poly commitment, eon=%d", dkg.Eon),
		shmsg.NewPolyCommitment(dkg.Eon, commitment.Gammas))
}

func (dcdr *Decider) startPhase2Accusing(dkg *DKG, phaseAtNextBlockHeight puredkg.Phase) {
	accusations := dkg.Pure.StartPhase2Accusing()
	if phaseAtNextBlockHeight != puredkg.Accusing {
		return
	}
	dcdr.sendShielderMessage(
		fmt.Sprintf("accusations, eon=%d, count=%d", dkg.Eon, len(accusations)),
		dkg.newAccusation(accusations))
}

func (dcdr *Decider) startPhase3Apologizing(dkg *DKG, phaseAtNextBlockHeight puredkg.Phase) {
	apologies := dkg.Pure.StartPhase3Apologizing()
	if phaseAtNextBlockHeight != puredkg.Apologizing {
		return
	}
	dcdr.sendShielderMessage(
		fmt.Sprintf("apologies, eon=%d, count=%d", dkg.Eon, len(apologies)),
		dkg.newApology(apologies))
}

func (dcdr *Decider) dkgFinalize(dkg *DKG) {
	dkg.Pure.Finalize()
	dkgresult, err := dkg.Pure.ComputeResult()
	if err != nil {
		log.Printf("Error: DKG process failed for %s: %+v", dkg.ShortInfo(), err)
		dcdr.sendShielderMessage(
			"requesting DKG restart",
			shmsg.NewEonStartVote(dkg.StartBatchIndex),
		)
		return
	}
	log.Printf("Success: DKG process succeeded for %s", dkg.ShortInfo())
	ekg := &EKG{
		Eon:     dkg.Eon,
		Keypers: dkg.Keypers,
		EpochKG: epochkg.NewEpochKG(&dkgresult),
	}
	dcdr.State.EKGs = append(dcdr.State.EKGs, ekg)
	dcdr.broadcastEonPublicKey(&dkgresult, dkg.StartBatchIndex)
}

func (dcdr *Decider) broadcastEonPublicKey(dkgResult *puredkg.Result, startBatchIndex uint64) {
	action := fx.EonKeyBroadcast{
		KeyperIndex:     dkgResult.Keyper,
		StartBatchIndex: startBatchIndex,
		EonPublicKey:    dkgResult.PublicKey,
	}
	dcdr.addAction(&action)
}

func (dcdr *Decider) syncDKGWithEon(dkg *DKG, eon observe.Eon) {
	decrypt := func(encrypted []byte) ([]byte, error) {
		return dcdr.Config.EncryptionKey.Decrypt(encrypted, []byte(""), []byte(""))
	}
	syncHeight := dcdr.State.SyncHeight
	// We look at the next block's phase, because that is the first block that might make it
	// into the chain
	phaseAtNextBlockHeight := dcdr.PhaseLength.getPhaseAtHeight(dcdr.Shielder.CurrentBlock+1, eon.StartHeight)

	if dkg.Pure.Phase == puredkg.Off && phaseAtNextBlockHeight >= puredkg.Dealing {
		dcdr.startPhase1Dealing(dkg, phaseAtNextBlockHeight)
	}
	dkg.syncCommitments(syncHeight, eon)
	dkg.syncPolyEvals(syncHeight, eon, decrypt)

	if dkg.Pure.Phase == puredkg.Dealing && phaseAtNextBlockHeight >= puredkg.Accusing {
		dcdr.startPhase2Accusing(dkg, phaseAtNextBlockHeight)
	}
	dkg.syncAccusations(syncHeight, eon)

	if dkg.Pure.Phase == puredkg.Accusing && phaseAtNextBlockHeight >= puredkg.Apologizing {
		dcdr.startPhase3Apologizing(dkg, phaseAtNextBlockHeight)
	}
	dkg.syncApologies(syncHeight, eon)

	if dkg.Pure.Phase == puredkg.Apologizing && phaseAtNextBlockHeight >= puredkg.Finalized {
		dcdr.dkgFinalize(dkg)
	}
}

func (dcdr *Decider) handleDKGs() {
	for i := range dcdr.State.DKGs {
		dkg := &dcdr.State.DKGs[i]
		if dkg.IsFinalized() {
			continue
		}
		eon, err := dcdr.Shielder.FindEon(dkg.Eon)
		if err != nil {
			panic(err)
		}
		dcdr.syncDKGWithEon(dkg, *eon)
		dcdr.sendPolyEvals(dkg)
	}
}

func (dcdr *Decider) publishEpochSecretKeyShare(batchIndex uint64) {
	epoch := batchIndex
	eon, err := dcdr.Shielder.FindEonByBatchIndex(batchIndex)
	if err != nil {
		return
	}

	ekg, err := dcdr.State.FindEKGByEon(eon.Eon)
	if err != nil {
		log.Printf("Cannot publish epoch secret key for epoch %d in eon %d, no eon key", epoch, eon.Eon)
		return
	}
	dcdr.sendEpochSecretKeyShare(ekg.EpochKG, epoch)
}

func (dcdr *Decider) syncEKGWithEon(syncHeight int64, ekg *EKG, eon *observe.Eon) {
	shares := eon.GetEpochSecretKeyShares(syncHeight)
	for _, share := range shares {
		sender, err := medley.FindAddressIndex(ekg.Keypers, share.Sender)
		if err != nil {
			continue
		}
		// Ignore this epoch secret key share, if we already have the secret key
		if _, ok := ekg.EpochKG.SecretKeys[share.Epoch]; ok {
			continue
		}
		err = ekg.EpochKG.HandleEpochSecretKeyShare(
			&epochkg.EpochSecretKeyShare{
				Eon:    share.Eon,
				Epoch:  share.Epoch,
				Sender: uint64(sender),
				Share:  share.Share,
			},
		)
		if err != nil {
			log.Printf("Error while handling epoch secret key share: %+v", err)
			continue
		}
		if key, ok := ekg.EpochKG.SecretKeys[share.Epoch]; ok {
			log.Printf("Epoch secret key generated for epoch %d", share.Epoch)
			dcdr.decryptTransactions(key, share.Epoch)
			if !dcdr.executionTimeoutReachedOrInactive(share.Epoch) {
				dcdr.sendDecryptionSignature(share.Epoch)
			}
		}
	}
}

// Add a prefix to avoid accidentally signing data with special meaning in different context, in
// particular Ethereum transactions (c.f. EIP191 https://eips.ethereum.org/EIPS/eip-191).
var hashPrefix = []byte{0x19, 'd', 'e', 'c', 't', 'x'}

func transactionsHash(txs [][]byte) []byte {
	keccak := sha3.NewLegacyKeccak256()
	hash := make([]byte, keccak.Size())

	for _, tx := range txs {
		keccak.Reset()
		if _, err := keccak.Write(tx); err != nil {
			panic(err)
		}
		if _, err := keccak.Write(hash); err != nil {
			panic(err)
		}
		hash = keccak.Sum(nil)
	}
	return hash
}

// computeDecryptionSignatureHash computes a cryptographic hash over the encrypted transactions,
// the decrypted transactions, the batcher contracts address and the batch index.
// It's the same hash we compute in the KeyperSlasher.sol's verifyAuthorization.
func (dcdr *Decider) computeDecryptionSignatureHash(batchIndex uint64, cipherBatchHash, batchHash []byte) []byte {
	keccak := sha3.NewLegacyKeccak256()
	if _, err := keccak.Write(hashPrefix); err != nil {
		panic(err)
	}

	batchIndexBuffer := new(bytes.Buffer)
	err := binary.Write(batchIndexBuffer, binary.BigEndian, batchIndex)
	if err != nil {
		panic(err)
	}

	if _, err := keccak.Write(dcdr.Config.BatcherContractAddress.Bytes()); err != nil {
		panic(err)
	}
	if _, err := batchIndexBuffer.WriteTo(keccak); err != nil {
		panic(err)
	}
	if _, err := keccak.Write(cipherBatchHash); err != nil {
		panic(err)
	}
	if _, err := keccak.Write(batchHash); err != nil {
		panic(err)
	}
	return keccak.Sum(nil)
}

func (dcdr *Decider) decryptTransactions(key *shcrypto.EpochSecretKey, epoch uint64) {
	batchIndex := epoch
	batch, ok := dcdr.MainChain.Batches[batchIndex]
	if !ok {
		// We may run into this case if our main chain node is lagging behind or if the
		// batch is empty. XXX The former case is not being handled here.
		log.Printf("Batch missing for batch index=%d", batchIndex)
		batch = &observe.Batch{BatchIndex: batchIndex}
	}
	txs := batch.DecryptTransactions(key)
	decryptedBatchHash := transactionsHash(txs)
	hash := dcdr.computeDecryptionSignatureHash(batchIndex, batch.EncryptedBatchHash.Bytes(), decryptedBatchHash)

	stBatch := &Batch{
		BatchIndex:              batchIndex,
		DecryptionSignatureHash: hash,
		DecryptedTransactions:   txs,
		DecryptedBatchHash:      decryptedBatchHash,
	}
	dcdr.State.Batches[batchIndex] = stBatch
}

func (dcdr *Decider) sendDecryptionSignature(epoch uint64) {
	batchIndex := epoch
	stBatch, ok := dcdr.State.Batches[batchIndex]
	if !ok {
		log.Printf("Batch %d is missing", batchIndex)
		return
	}

	// Let's sync this with shielder to see if we still need to send a decryption message
	dcdr.syncBatch(stBatch)

	config, ok := dcdr.MainChain.ConfigForBatchIndex(batchIndex)
	if !ok {
		log.Panicf("no main chain config for batch %d", batchIndex)
	}

	if uint64(stBatch.SignatureCount) < config.Threshold && len(stBatch.DecryptedTransactions) > 0 {
		signature, err := crypto.Sign(stBatch.DecryptionSignatureHash, dcdr.Config.SigningKey)
		if err != nil {
			log.Panicf("Cannot sign the decryption signature: %s", err)
		}

		decryptionSignature := shmsg.NewDecryptionSignature(batchIndex, signature)
		dcdr.sendShielderMessage(
			fmt.Sprintf("decryption signature, batch index=%d", batchIndex),
			decryptionSignature)
	}
}

func (dcdr *Decider) syncEKGs() {
	for _, ekg := range dcdr.State.EKGs {
		eon, err := dcdr.Shielder.FindEon(ekg.Eon)
		if err != nil {
			panic(err)
		}
		dcdr.syncEKGWithEon(dcdr.State.SyncHeight, ekg, eon)
	}
}

func (dcdr *Decider) publishEpochSecretKeyShares() {
	blockNum := dcdr.MainChain.CurrentBlock

	// Find the active config for the given block on the main chain
	activeCFGIdx := dcdr.MainChain.ActiveConfigIndex(blockNum)
	bc := dcdr.MainChain.BatchConfigs[activeCFGIdx]

	// find the corresponding config on shielder
	_, err := dcdr.Shielder.FindBatchConfigByConfigIndex(uint64(activeCFGIdx))
	if err != nil {
		return
	}

	currentBatchIndex := bc.BatchIndex(blockNum)
	// publish the private epoch key share for batch indexes < currentBatchIndex
	for batchIndex := dcdr.State.NextEpochSecretShare; batchIndex < currentBatchIndex; batchIndex++ {
		if !dcdr.executionTimeoutReachedOrInactive(batchIndex) {
			dcdr.publishEpochSecretKeyShare(batchIndex)
		}
	}
	dcdr.State.NextEpochSecretShare = currentBatchIndex
}

// executionTimeoutReachedOrInactive checks if the execution timeout for the given batch has been reached or
// if the config is inactive.
func (dcdr *Decider) executionTimeoutReachedOrInactive(batchIndex uint64) bool {
	config, ok := dcdr.MainChain.ConfigForBatchIndex(batchIndex)
	if !ok {
		return true // config is inactive
	}
	executionTimeoutBlock := config.BatchEndBlock(batchIndex) + config.ExecutionTimeout
	return dcdr.MainChain.CurrentBlock >= executionTimeoutBlock-1
}

func (dcdr *Decider) handleEpochKG() {
	dcdr.syncEKGs()
	dcdr.publishEpochSecretKeyShares()
}

func (dcdr *Decider) sendEpochSecretKeyShare(epochKG *epochkg.EpochKG, epoch uint64) {
	if _, ok := epochKG.SecretKeys[epoch]; !ok {
		epochSecretKeyShare := epochKG.ComputeEpochSecretKeyShare(epoch)
		dcdr.sendShielderMessage(
			fmt.Sprintf("epoch secret key share, epoch=%d in eon=%d", epoch, epochKG.Eon),
			shmsg.NewEpochSecretKeyShare(epochKG.Eon, epoch, epochSecretKeyShare),
		)
	}
}

func (dcdr *Decider) syncBatch(batch *Batch) {
	shBatch, ok := dcdr.Shielder.Batches[batch.BatchIndex]
	if !ok {
		return
	}
	config, ok := dcdr.MainChain.ConfigForBatchIndex(batch.BatchIndex)
	if !ok {
		panic("Error in syncBatch: config is not active")
	}

	if len(batch.DecryptedTransactions) == 0 {
		return
	}

	signatureCount := 0
	for i := batch.DecryptionSignatureIndex; i < len(shBatch.DecryptionSignatures); i++ {
		ev := shBatch.DecryptionSignatures[i]
		if !config.IsKeyper(ev.Sender) {
			log.Printf("Error: received signature for batch %d from non-keyper %s", batch.BatchIndex, ev.Sender.Hex())
			continue
		}
		if batch.VerifySignature(ev.Sender, ev.Signature) {
			signatureCount++
		} else {
			log.Printf("Bad signature from %s for batch %d", ev.Sender.Hex(), batch.BatchIndex)
		}
	}
	if signatureCount > 0 {
		batch.SignatureCount += signatureCount
		log.Printf("Verified %d signatures for batch %d, total %d signatures", signatureCount, batch.BatchIndex, batch.SignatureCount)
	}
	batch.DecryptionSignatureIndex = len(shBatch.DecryptionSignatures)
}

func (dcdr *Decider) handleDecryptionSignatures() {
	for _, batch := range dcdr.State.Batches {
		dcdr.syncBatch(batch)
	}
}

func (dcdr *Decider) maybeExecuteBatch() {
	config := dcdr.MainChain.CurrentConfig()
	if !config.IsActive() {
		return // nothing to execute if config is inactive
	}
	batchIndex := config.BatchIndex(dcdr.MainChain.CurrentBlock)

	nextHalfStep := dcdr.MainChain.NumExecutionHalfSteps
	if dcdr.State.PendingHalfStep != nil && nextHalfStep > *dcdr.State.PendingHalfStep {
		// Reset the pending half step if the current one is greater.
		// XXX There's a chance that another keyper has executed the previous half step and our tx
		// is still pending. In that case we should probably wait until it fails before sending
		// another one.
		dcdr.State.PendingHalfStep = nil
	}
	if dcdr.State.PendingHalfStep != nil {
		// Don't try to execute anything if there's already one or more pending transaction
		// executing the current or another half step. Rather, wait for them to confirm first.
		return
	}

	numHalfStepsToExecute := getNumHalfStepsToExecute(nextHalfStep, batchIndex)
	for halfStep := nextHalfStep; halfStep < nextHalfStep+numHalfStepsToExecute; halfStep++ {
		if action := dcdr.maybeExecuteHalfStep(halfStep); action != nil {
			dcdr.addAction(action)
			halfStep2 := halfStep // avoid using reference to loop variable
			dcdr.State.PendingHalfStep = &halfStep2
		} else {
			break
		}
	}
}

// getNumHalfStepsToExecute returns the number of half steps to execute given the index of the
// next half step to execute (i.e., the total number of already executed half steps) and the
// current batch index.
func getNumHalfStepsToExecute(nextHalfStep uint64, batchIndex uint64) uint64 {
	if nextHalfStep >= batchIndex*2 {
		return 0
	}
	numMissingHalfSteps := batchIndex*2 - nextHalfStep
	if numMissingHalfSteps <= maxParallelHalfSteps {
		return numMissingHalfSteps
	}
	return maxParallelHalfSteps
}

func (dcdr *Decider) executeCipherBatch(batchIndex uint64, config contract.BatchConfig) fx.IAction {
	batch, ok := dcdr.MainChain.Batches[batchIndex]
	if !ok {
		batch = &observe.Batch{BatchIndex: batchIndex}
	}

	keyperIndex, ok := config.KeyperIndex(dcdr.Config.Address())
	if !ok {
		log.Fatal("executeCipherBatch called from non keyper")
	}
	stBatch, ok := dcdr.State.Batches[batchIndex]
	if !ok {
		log.Printf("Error: no data for batch %d", batchIndex)
		return nil
	}

	if len(stBatch.DecryptedTransactions) > 0 && uint64(stBatch.SignatureCount) < config.Threshold {
		log.Printf("not enough votes for batch %d", batchIndex)
		return nil
	}

	return &fx.ExecuteCipherBatch{
		BatchIndex:          batchIndex,
		CipherBatchHash:     batch.EncryptedBatchHash,
		Transactions:        stBatch.DecryptedTransactions,
		KeyperIndex:         keyperIndex,
		TransactionGasLimit: config.TransactionGasLimit,
	}
}

func (dcdr *Decider) executePlainBatch(batchIndex uint64, config contract.BatchConfig) fx.IAction {
	batch, ok := dcdr.MainChain.Batches[batchIndex]
	if !ok {
		batch = &observe.Batch{BatchIndex: batchIndex}
	}
	return &fx.ExecutePlainBatch{
		BatchIndex:          batchIndex,
		Transactions:        batch.PlainTransactions,
		TransactionGasLimit: config.TransactionGasLimit,
	}
}

func (dcdr *Decider) maybeExecuteHalfStep(nextHalfStep uint64) fx.IAction {
	batchIndex := nextHalfStep / 2

	config, ok := dcdr.MainChain.ConfigForBatchIndex(batchIndex)
	if !ok {
		return nil // nothing to do if config is inactive
	}

	delay := dcdr.executionDelay(config, nextHalfStep)
	executionBlock := config.BatchEndBlock(batchIndex) + delay
	executionTimeoutBlock := config.BatchEndBlock(batchIndex) + config.ExecutionTimeout
	isCipherBatch := nextHalfStep%2 == 0

	// skip cipher half steps if execution timeout block + delay is passed
	if isCipherBatch && dcdr.MainChain.CurrentBlock >= executionTimeoutBlock {
		if dcdr.MainChain.CurrentBlock >= executionTimeoutBlock+delay {
			return &fx.SkipCipherBatch{
				BatchIndex: batchIndex,
			}
		}
		return nil
	}

	if !config.IsKeyper(dcdr.Config.Address()) {
		// we can't execute this batch
		return nil
	}

	// execute batch if execution block is passed
	if dcdr.MainChain.CurrentBlock >= executionBlock {
		if isCipherBatch {
			return dcdr.executeCipherBatch(batchIndex, config)
		}
		return dcdr.executePlainBatch(batchIndex, config)
	}
	return nil
}

// maybeAppeal checks if there are any accusations against anyone and if so sends an appeal if
// possible.
func (dcdr *Decider) maybeAppeal() {
	dcdr.syncPendingAppeals()

	for _, accusation := range dcdr.MainChain.Accusations {
		if accusation.Appealed {
			continue
		}
		if _, ok := dcdr.State.PendingAppeals[accusation.HalfStep]; ok {
			continue // don't send appeal if we've already done so and the tx is still pending
		}

		batchIndex := accusation.HalfStep / 2
		signatures, indices, err := dcdr.Shielder.GetSortedDecryptionSignaturesWithIndices(batchIndex)
		if err != nil {
			log.Printf("Error: cannot appeal accusation because getting decryption signatures for batch %d failed", batchIndex)
			return
		}

		batchConfig := dcdr.Shielder.FindBatchConfigByBatchIndex(batchIndex)
		if err != nil {
			log.Printf("Error: cannot appeal accusation because getting batch config for batch %d failed", batchIndex)
			return
		}

		if uint64(len(signatures)) < batchConfig.Threshold {
			log.Printf(
				"Error: cannot appeal accusation because of not enough signatures (%d at a threshold of %d)",
				len(signatures),
				batchConfig.Threshold,
			)
			return
		}

		stBatch, ok := dcdr.State.Batches[batchIndex]
		if !ok {
			log.Printf("Error: cannot appeal because batch %d is missing", batchIndex)
			return
		}
		batchHashSlice := transactionsHash(stBatch.DecryptedTransactions)
		batchHash := [32]byte{}
		copy(batchHash[:], batchHashSlice)

		authorization := contract.Authorization{
			HalfStep:      accusation.HalfStep,
			BatchHash:     batchHash,
			SignerIndices: indices,
			Signatures:    contract.SignaturesToContractFormat(signatures),
		}
		action := fx.Appeal{
			Authorization: authorization,
		}
		dcdr.State.PendingAppeals[accusation.HalfStep] = struct{}{}
		dcdr.addAction(&action)
	}
}

func (dcdr *Decider) maybeAccuse() {
	for halfStep := dcdr.State.HalfStepsChecked; halfStep < dcdr.MainChain.NumExecutionHalfSteps; halfStep++ {
		if halfStep%2 != 0 {
			continue // only accuse for cipher execution half steps, not plain ones
		}

		batchIndex := halfStep / 2

		receipt := dcdr.MainChain.CipherExecutionReceipts[halfStep]
		if receipt == nil {
			// half step was skipped
			continue
		}

		stBatch, ok := dcdr.State.Batches[batchIndex]
		if !ok {
			continue
		}

		if !bytes.Equal(stBatch.DecryptedBatchHash, receipt.BatchHash[:]) {
			// what was decrypted does not match what we've decrypted

			if _, ok := dcdr.MainChain.Accusations[halfStep]; ok {
				log.Printf("Not accusing executor for batch %d because accusation already present", batchIndex)
				continue
			}

			config, ok := dcdr.MainChain.ConfigForBatchIndex(batchIndex)
			if !ok {
				log.Printf("Error: cannot accuse executor of batch %d because config is missing", batchIndex)
				continue
			}

			keyperIndex, _ := config.KeyperIndex(dcdr.Config.Address())
			action := fx.Accuse{
				HalfStep:    halfStep,
				KeyperIndex: keyperIndex,
			}
			dcdr.addAction(&action)
		}
	}

	dcdr.State.HalfStepsChecked = dcdr.MainChain.NumExecutionHalfSteps
}

// syncPendingAppeals removes any pending appeals that have been successfully handled by the main
// chain.
// XXX: It's possible that someone else appeals, in which case our tx would still be pending.
// Also, we don't notice if our tx fails (but this shouldn't happen if we prepare it properly).
func (dcdr *Decider) syncPendingAppeals() {
	for halfStep := range dcdr.State.PendingAppeals {
		for _, accusation := range dcdr.MainChain.Accusations {
			if halfStep == accusation.HalfStep && accusation.Appealed {
				delete(dcdr.State.PendingAppeals, halfStep)
			}
		}
	}
}

// executionDelay returns the number of main chain blocks to wait before sending an execution tx.
// This makes sure not all keypers try to send the same tx at the same time.
func (dcdr *Decider) executionDelay(config contract.BatchConfig, halfStep uint64) uint64 {
	// do not delay the execution for more than 50% of the execution timeout
	var maxStaggering, staggering, divisor uint64
	divisor = uint64(len(config.Keypers) - 1)
	if divisor == 0 {
		maxStaggering = 0
	} else {
		maxStaggering = config.ExecutionTimeout / (2 * divisor)
	}

	if dcdr.Config.ExecutionStaggering >= maxStaggering {
		staggering = maxStaggering
	} else {
		staggering = dcdr.Config.ExecutionStaggering
	}

	keyperIndex, _ := config.KeyperIndex(dcdr.Config.Address())
	place := (halfStep + keyperIndex) % uint64(len(config.Keypers))

	return place * staggering
}

// Decide determines the next actions to run.
func (dcdr *Decider) Decide() {
	if !dcdr.Shielder.IsSynced() {
		log.Printf("shuttermint chain out of sync, waiting")
		return
	}
	if !dcdr.MainChain.IsSynced() {
		log.Printf("main chain out of sync, waiting")
		return
	}
	// We can't go on unless we're registered as keyper in shuttermint
	if !dcdr.Shielder.IsKeyper(dcdr.Config.Address()) {
		log.Printf("not registered as keyper in shuttermint, nothing to do")
		return
	}
	dcdr.maybeSendCheckIn()
	dcdr.maybeSendBatchConfig()
	dcdr.maybeStartDKG()
	dcdr.handleDKGs()
	dcdr.handleEpochKG()
	dcdr.handleDecryptionSignatures()
	dcdr.maybeExecuteBatch()
	dcdr.maybeAppeal()
	dcdr.maybeAccuse()
	dcdr.State.SyncHeight = dcdr.Shielder.CurrentBlock + 1
}
