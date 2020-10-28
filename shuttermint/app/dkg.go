package app

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// NewDKGInstance creates a new DKGInstance.
func NewDKGInstance(config BatchConfig) DKGInstance {
	polyEvalMsgs := make(map[common.Address]map[common.Address]PolyEvalMsg)
	polyCommitmentMsgs := make(map[common.Address]PolyCommitmentMsg)
	accusationMsgs := make(map[common.Address]map[common.Address]AccusationMsg)
	apologyMsgs := make(map[common.Address]map[common.Address]ApologyMsg)

	for _, keyper := range config.Keypers {
		polyEvalMsgs[keyper] = make(map[common.Address]PolyEvalMsg)
		accusationMsgs[keyper] = make(map[common.Address]AccusationMsg)
		apologyMsgs[keyper] = make(map[common.Address]ApologyMsg)
	}

	return DKGInstance{
		Config: config,

		PolyEvalMsgs:       polyEvalMsgs,
		PolyCommitmentMsgs: polyCommitmentMsgs,
		AccusationMsgs:     accusationMsgs,
		ApologyMsgs:        apologyMsgs,
	}
}

// RegisterPolyEvalMsg adds a polynomial evaluation message to the instance.
func (dkg *DKGInstance) RegisterPolyEvalMsg(msg PolyEvalMsg) error {
	if !dkg.Config.IsKeyper(msg.Sender) {
		return fmt.Errorf("sender %s is not a keyper", msg.Sender.Hex())
	}
	if !dkg.Config.IsKeyper(msg.Receiver) {
		return fmt.Errorf("receiver %s is not a keyper", msg.Sender.Hex())
	}
	if msg.Sender == msg.Receiver {
		return fmt.Errorf("sender and receiver are both %s", msg.Sender.Hex())
	}

	dkg.Lock()
	defer dkg.Unlock()

	if _, ok := dkg.PolyEvalMsgs[msg.Sender][msg.Receiver]; ok {
		return fmt.Errorf("polynomial evaluation from keyper %s for keyper %s already present", msg.Sender.Hex(), msg.Receiver.Hex())
	}
	dkg.PolyEvalMsgs[msg.Sender][msg.Receiver] = msg

	return nil
}

// RegisterPolyCommitmentMsg adds a polynomial commitment message to the instance.
func (dkg *DKGInstance) RegisterPolyCommitmentMsg(msg PolyCommitmentMsg) error {
	if !dkg.Config.IsKeyper(msg.Sender) {
		return fmt.Errorf("sender %s is not a keyper", msg.Sender.Hex())
	}

	dkg.Lock()
	defer dkg.Unlock()

	if _, ok := dkg.PolyCommitmentMsgs[msg.Sender]; ok {
		return fmt.Errorf("polynomial commitment from keyper %s already present", msg.Sender.Hex())
	}
	dkg.PolyCommitmentMsgs[msg.Sender] = msg

	return nil
}

// RegisterAccusationMsg adds an accusation message to the instance.
func (dkg *DKGInstance) RegisterAccusationMsg(msg AccusationMsg) error {
	if !dkg.Config.IsKeyper(msg.Sender) {
		return fmt.Errorf("sender %s is not a keyper", msg.Sender.Hex())
	}
	if !dkg.Config.IsKeyper(msg.Accused) {
		return fmt.Errorf("accused %s is not a keyper", msg.Sender.Hex())
	}
	if msg.Sender == msg.Accused {
		return fmt.Errorf("sender and accused are both %s", msg.Sender.Hex())
	}

	dkg.Lock()
	defer dkg.Unlock()

	if _, ok := dkg.AccusationMsgs[msg.Sender][msg.Accused]; ok {
		return fmt.Errorf("accusation from keyper %s against %s already present", msg.Sender.Hex(), msg.Accused.Hex())
	}
	dkg.AccusationMsgs[msg.Sender][msg.Accused] = msg

	return nil
}

// RegisterApologyMsg adds an apology message to the instance.
func (dkg *DKGInstance) RegisterApologyMsg(msg ApologyMsg) error {
	if !dkg.Config.IsKeyper(msg.Sender) {
		return fmt.Errorf("sender %s is not a keyper", msg.Sender.Hex())
	}
	if !dkg.Config.IsKeyper(msg.Accuser) {
		return fmt.Errorf("accuser %s is not a keyper", msg.Sender.Hex())
	}
	if msg.Sender == msg.Accuser {
		return fmt.Errorf("sender and accuser are both %s", msg.Sender.Hex())
	}

	dkg.Lock()
	defer dkg.Unlock()

	if _, ok := dkg.ApologyMsgs[msg.Sender][msg.Accuser]; ok {
		return fmt.Errorf("apology from keyper %s against apology of %s already present", msg.Sender.Hex(), msg.Accuser.Hex())
	}
	dkg.ApologyMsgs[msg.Sender][msg.Accuser] = msg

	return nil
}
