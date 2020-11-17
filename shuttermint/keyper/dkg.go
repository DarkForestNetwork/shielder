package keyper

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/ecies"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/crypto"
)

// DKGInstance represents the state of a single keyper participating in a DKG process.
type DKGInstance struct {
	Eon          uint64
	BatchConfig  contract.BatchConfig
	KeyperConfig KeyperConfig

	ms                   MessageSender
	keyperEncryptionKeys map[common.Address]*ecies.PublicKey

	Polynomial *crypto.Polynomial
	Commitment map[common.Address]crypto.Gammas
}

// NewDKGInstance creates a new dkg instance with initialized local random values.
func NewDKGInstance(
	eon uint64,
	batchConfig contract.BatchConfig,
	keyperConfig KeyperConfig,
	ms MessageSender,
	keyperEncryptionKeys map[common.Address]*ecies.PublicKey,
) (*DKGInstance, error) {
	polynomial, err := crypto.RandomPolynomial(rand.Reader, batchConfig.Threshold)
	if err != nil {
		return nil, err
	}

	dkg := DKGInstance{
		Eon:          eon,
		BatchConfig:  batchConfig,
		KeyperConfig: keyperConfig,

		ms:                   ms,
		keyperEncryptionKeys: keyperEncryptionKeys,

		Polynomial: polynomial,
		Commitment: make(map[common.Address]crypto.Gammas),
	}
	return &dkg, nil
}

// Run everything.
func (dkg *DKGInstance) Run(ctx context.Context) error {
	err := dkg.sendGammas(ctx)
	if err != nil {
		return err
	}
	err = dkg.sendPolyEvals(ctx)
	if err != nil {
		return err
	}

	return nil
}

// sendGammas broadcasts the gamma values.
func (dkg *DKGInstance) sendGammas(ctx context.Context) error {
	msg := NewPolyCommitmentMsg(dkg.Eon, dkg.Polynomial.Gammas())
	return dkg.ms.SendMessage(ctx, msg)
}

// sendPolyEvals sends the corresponding polynomial evaluation to each keyper, including ourselves.
func (dkg *DKGInstance) sendPolyEvals(ctx context.Context) error {
	for i, keyper := range dkg.BatchConfig.Keypers {
		encryptionKey, ok := dkg.keyperEncryptionKeys[keyper]
		if !ok {
			continue // don't send them a message if there encryption public key is unknown
		}

		eval := dkg.Polynomial.EvalForKeyper(i)
		evalBytes := eval.Bytes()
		encryptedEval, err := ecies.Encrypt(rand.Reader, encryptionKey, evalBytes, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to encrypt message: %w", err)
		}

		msg := NewPolyEvalMsg(dkg.Eon, keyper, encryptedEval)
		err = dkg.ms.SendMessage(ctx, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dkg *DKGInstance) dispatchShielderEvent(ev IEvent) {
	switch e := ev.(type) {
	case PolyCommitmentRegisteredEvent:
		// XXX we should handle the case where we already have a commitment
		dkg.Commitment[e.Sender] = *e.Gammas
	default:
		panic("unknown event type, cannot dispatch")
	}
}
