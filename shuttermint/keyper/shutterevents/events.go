// Package shielderevents contains types to represent deserialized shuttermint/tendermint events
package shielderevents

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	bn256 "github.com/ethereum/go-ethereum/crypto/bn256/cloudflare"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	abcitypes "github.com/tendermint/tendermint/abci/types"

	"shielder/shuttermint/app"
	"shielder/shuttermint/app/evtype"
	"shielder/shuttermint/crypto"
)

// CheckInEvent is emitted by shuttermint when a keyper sends their check in message.
type CheckInEvent struct {
	Sender              common.Address
	EncryptionPublicKey *ecies.PublicKey
}

// BatchConfigEvent is generated by shuttermint, when a new BatchConfig has been added
type BatchConfigEvent struct {
	StartBatchIndex uint64
	Threshold       uint64
	Keypers         []common.Address
	ConfigIndex     uint64
}

// DecryptionSignatureEvent is generated by shuttermint when a keyper sends a decryption
// signature.
type DecryptionSignatureEvent struct {
	BatchIndex uint64
	Sender     common.Address
	Signature  []byte
}

// EonStarted is generated by shuttermint when a new eon is started.
type EonStartedEvent struct {
	Eon        uint64
	BatchIndex uint64
}

// PolyCommitmentRegisteredEvent
type PolyCommitmentRegisteredEvent struct {
	Eon    uint64
	Sender common.Address
	Gammas *crypto.Gammas
}

// PolyEvalRegisteredEvent
type PolyEvalRegisteredEvent struct {
	Eon            uint64
	Sender         common.Address
	Receivers      []common.Address
	EncryptedEvals [][]byte
}

// IEvent is an interface for the event types declared above
type IEvent interface {
	IEvent()
}

func (CheckInEvent) IEvent()                  {}
func (BatchConfigEvent) IEvent()              {}
func (DecryptionSignatureEvent) IEvent()      {}
func (EonStartedEvent) IEvent()               {}
func (PolyCommitmentRegisteredEvent) IEvent() {}
func (PolyEvalRegisteredEvent) IEvent()       {}

func getBytesAttribute(ev abcitypes.Event, index int, key string) ([]byte, error) {
	if len(ev.Attributes) <= index {
		return []byte{}, fmt.Errorf("event does not have enough attributes")
	}
	attr := ev.Attributes[index]
	if string(attr.Key) != key {
		return []byte{}, fmt.Errorf("expected attribute key %s at index %d, got %s", key, index, attr.Key)
	}
	return attr.Value, nil
}

func getUint64Attribute(ev abcitypes.Event, index int, name string) (uint64, error) {
	attr, err := getBytesAttribute(ev, index, name)
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(string(attr))
	if err != nil {
		return 0, fmt.Errorf("failed to parse event: %w", err)
	}
	return uint64(v), nil
}

func decodeGammasFromEvent(eventValue []byte) (crypto.Gammas, error) {
	parts := strings.Split(string(eventValue), ",")
	var res crypto.Gammas
	for _, p := range parts {
		marshaledG2, err := hex.DecodeString(p)
		if err != nil {
			return crypto.Gammas{}, err
		}
		g := new(bn256.G2)
		_, err = g.Unmarshal(marshaledG2)
		if err != nil {
			return crypto.Gammas{}, err
		}
		res = append(res, g)
	}
	return res, nil
}

func getGammasAttribute(ev abcitypes.Event, index int, name string) (crypto.Gammas, error) {
	attr, err := getBytesAttribute(ev, index, name)
	if err != nil {
		return crypto.Gammas{}, err
	}
	return decodeGammasFromEvent(attr)
}

func getStringAttribute(ev abcitypes.Event, index int, key string) (string, error) {
	b, err := getBytesAttribute(ev, index, key)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func getAddressAttribute(ev abcitypes.Event, index int, key string) (common.Address, error) {
	s, err := getStringAttribute(ev, index, key)
	if err != nil {
		return common.Address{}, err
	}
	a := common.HexToAddress(s)
	if a.Hex() != s {
		return common.Address{}, fmt.Errorf("invalid address %s", s)
	}
	return a, nil
}

func getPublicKeyAttribute(ev abcitypes.Event, index int, key string) (*ecdsa.PublicKey, error) {
	s, err := getStringAttribute(ev, index, key)
	if err != nil {
		return nil, err
	}

	publicKey, err := app.DecodePubkeyFromEvent(s)
	if err != nil {
		return nil, err
	}

	return publicKey, nil
}

func getECIESPublicKeyAttribute(ev abcitypes.Event, index int, key string) (*ecies.PublicKey, error) {
	publicKeyECDSA, err := getPublicKeyAttribute(ev, index, key)
	if err != nil {
		return nil, err
	}
	return ecies.ImportECDSAPublic(publicKeyECDSA), nil
}

// makeCheckInEvent creates a CheckInEvent from the given tendermint event of type "shielder.check-in"
func makeCheckInEvent(ev abcitypes.Event) (CheckInEvent, error) {
	if ev.Type != evtype.CheckIn {
		return CheckInEvent{}, fmt.Errorf("expected event type shielder.check-in, got %s", ev.Type)
	}

	sender, err := getAddressAttribute(ev, 0, "Sender")
	if err != nil {
		return CheckInEvent{}, err
	}
	publicKey, err := getECIESPublicKeyAttribute(ev, 1, "EncryptionPublicKey")
	if err != nil {
		return CheckInEvent{}, err
	}

	return CheckInEvent{
		Sender:              sender,
		EncryptionPublicKey: publicKey,
	}, nil
}

// makeBatchConfigEvent creates a BatchConfigEvent from the given tendermint event of type
// "shielder.batch-config"
func makeBatchConfigEvent(ev abcitypes.Event) (BatchConfigEvent, error) {
	if len(ev.Attributes) < 4 {
		return BatchConfigEvent{}, fmt.Errorf("event contains not enough attributes: %+v", ev)
	}
	if !bytes.Equal(ev.Attributes[0].Key, []byte("StartBatchIndex")) ||
		!bytes.Equal(ev.Attributes[1].Key, []byte("Threshold")) ||
		!bytes.Equal(ev.Attributes[2].Key, []byte("Keypers")) ||
		!bytes.Equal(ev.Attributes[3].Key, []byte("ConfigIndex")) {
		return BatchConfigEvent{}, fmt.Errorf("bad event attributes: %+v", ev)
	}

	b, err := strconv.Atoi(string(ev.Attributes[0].Value))
	if err != nil {
		return BatchConfigEvent{}, err
	}

	threshold, err := strconv.Atoi(string(ev.Attributes[1].Value))
	if err != nil {
		return BatchConfigEvent{}, err
	}
	keypers := app.DecodeAddressesFromEvent(string(ev.Attributes[2].Value))
	configIndex, err := strconv.ParseUint(string(ev.Attributes[3].Value), 10, 64)
	if err != nil {
		return BatchConfigEvent{}, err
	}
	return BatchConfigEvent{uint64(b), uint64(threshold), keypers, configIndex}, nil
}

// makeDecryptionSignatureEvent creates a DecryptionSignatureEvent from the given tendermint event
// of type "shielder.decryption-signature".
func makeDecryptionSignatureEvent(ev abcitypes.Event) (DecryptionSignatureEvent, error) {
	if len(ev.Attributes) < 3 {
		return DecryptionSignatureEvent{}, fmt.Errorf("event contains not enough attributes: %+v", ev)
	}
	if !bytes.Equal(ev.Attributes[0].Key, []byte("BatchIndex")) ||
		!bytes.Equal(ev.Attributes[1].Key, []byte("Sender")) ||
		!bytes.Equal(ev.Attributes[2].Key, []byte("Signature")) {
		return DecryptionSignatureEvent{}, fmt.Errorf("bad event attributes: %+v", ev)
	}

	batchIndex, err := strconv.Atoi(string(ev.Attributes[0].Value))
	if err != nil {
		return DecryptionSignatureEvent{}, err
	}

	encodedSender := string(ev.Attributes[1].Value)
	sender := common.HexToAddress(encodedSender)
	if sender.Hex() != encodedSender {
		return DecryptionSignatureEvent{}, fmt.Errorf("invalid sender address %s", encodedSender)
	}

	signature, err := base64.RawURLEncoding.DecodeString(string(ev.Attributes[2].Value))
	if err != nil {
		return DecryptionSignatureEvent{}, err
	}

	return DecryptionSignatureEvent{
		BatchIndex: uint64(batchIndex),
		Sender:     sender,
		Signature:  signature,
	}, nil
}

// makeEonStartedEvent creates a EonStartedEvent from the given tendermint event of type
// "shielder.eon-started".
func makeEonStartedEvent(ev abcitypes.Event) (EonStartedEvent, error) {
	if ev.Type != evtype.EonStarted {
		return EonStartedEvent{}, fmt.Errorf("expected event type %s, got %s", evtype.EonStarted, ev.Type)
	}

	eon, err := getUint64Attribute(ev, 0, "Eon")
	if err != nil {
		return EonStartedEvent{}, err
	}
	batchIndex, err := getUint64Attribute(ev, 1, "BatchIndex")
	if err != nil {
		return EonStartedEvent{}, err
	}

	return EonStartedEvent{
		Eon:        eon,
		BatchIndex: batchIndex,
	}, nil
}

func makePolyCommitmentRegisteredEvent(ev abcitypes.Event) (PolyCommitmentRegisteredEvent, error) {
	res := PolyCommitmentRegisteredEvent{}
	if ev.Type != evtype.PolyCommitment {
		return res, fmt.Errorf("expected event type shielder.poly-commitment-registered, got %s", ev.Type)
	}

	sender, err := getAddressAttribute(ev, 0, "Sender")
	if err != nil {
		return res, err
	}
	res.Sender = sender

	eon, err := getUint64Attribute(ev, 1, "Eon")
	if err != nil {
		return res, err
	}
	res.Eon = eon

	gammas, err := getGammasAttribute(ev, 2, "Gammas")
	if err != nil {
		return res, err
	}
	res.Gammas = &gammas

	return res, nil
}

func makePolyEvalRegisteredEvent(ev abcitypes.Event) (PolyEvalRegisteredEvent, error) {
	res := PolyEvalRegisteredEvent{}
	if ev.Type != evtype.PolyEval {
		return res, fmt.Errorf("expected event type shielder.poly-eval-registered, got %s", ev.Type)
	}

	sender, err := getAddressAttribute(ev, 0, "Sender")
	if err != nil {
		return res, err
	}
	res.Sender = sender

	eon, err := getUint64Attribute(ev, 1, "Eon")
	if err != nil {
		return res, err
	}
	res.Eon = eon

	return res, nil
}

// MakeEvent creates an Event from the given tendermint event.
func MakeEvent(ev abcitypes.Event) (IEvent, error) {
	switch ev.Type {
	case evtype.CheckIn:
		return makeCheckInEvent(ev)
	case evtype.BatchConfig:
		return makeBatchConfigEvent(ev)
	case evtype.DecryptionSignature:
		return makeDecryptionSignatureEvent(ev)
	case evtype.EonStarted:
		return makeEonStartedEvent(ev)
	case evtype.PolyCommitment:
		return makePolyCommitmentRegisteredEvent(ev)
	case evtype.PolyEval:
		return makePolyEvalRegisteredEvent(ev)
	default:
		return nil, fmt.Errorf("cannot make event from type %s", ev.Type)
	}
}
