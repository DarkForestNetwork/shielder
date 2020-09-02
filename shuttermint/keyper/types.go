package keyper

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"shielder/shuttermint/app"
	"shielder/shuttermint/shmsg"
	"github.com/ethereum/go-ethereum/common"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/types"
)

// RoundInterval is the duration between the start of two consecutive rounds
var RoundInterval time.Duration = time.Duration(5 * time.Second)

// PrivateKeyDeley is the duration between the start of the public key generation and the the start
// of the private key generation for a single round
var PrivateKeyDelay time.Duration = time.Duration(45 * time.Second)

type BatchParams struct {
	BatchIndex                   uint64
	PublicKeyGenerationStartTime time.Time
	// PublicKeyGenerationDuration  time.Duration
	PrivateKeyGenerationStartTime time.Time
}

type Keyper struct {
	SigningKey     *ecdsa.PrivateKey
	ShielderURL string
}

func NewBatchParams(BatchIndex uint64) BatchParams {
	ts := int64(BatchIndex) * int64(RoundInterval)

	pubstart := time.Unix(ts/int64(time.Second), ts%int64(time.Second))
	privstart := pubstart.Add(PrivateKeyDelay)
	return BatchParams{
		BatchIndex:                    BatchIndex,
		PublicKeyGenerationStartTime:  pubstart,
		PrivateKeyGenerationStartTime: privstart,
	}
}

func NextBatchIndex(t time.Time) uint64 {
	return uint64((t.UnixNano() + int64(RoundInterval) - 1) / int64(RoundInterval))
}

type BatchRunner struct {
	rpcclient client.Client
	params    BatchParams
}

type MessageSender struct {
	rpcclient  client.Client
	signingKey *ecdsa.PrivateKey
}

func NewMessageSender(client client.Client, signingKey *ecdsa.PrivateKey) MessageSender {
	return MessageSender{client, signingKey}
}

func (ms MessageSender) SendMessage(msg *shmsg.Message) error {
	signedMessage, err := shmsg.SignMessage(msg, ms.signingKey)
	if err != nil {
		return err
	}
	var tx types.Tx = types.Tx(base64.RawURLEncoding.EncodeToString(signedMessage))
	res, err := ms.rpcclient.BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	// fmt.Println("broadcast tx", res)
	if res.DeliverTx.Code != 0 {
		return fmt.Errorf("Error in SendMessage: %s", res.DeliverTx.Log)
	}
	return nil
}

type PubkeyGeneratedEvent struct {
	BatchIndex uint64
	Pubkey     *ecdsa.PublicKey
}

type PrivkeyGeneratedEvent struct {
	BatchIndex uint64
	Privkey    *ecdsa.PrivateKey
}

type BatchConfigEvent struct {
	StartBatchIndex uint64
	Threshhold      uint32
	Keypers         []common.Address
}

func MakePrivkeyGeneratedEvent(ev abcitypes.Event) (PrivkeyGeneratedEvent, error) {
	if len(ev.Attributes) < 2 {
		return PrivkeyGeneratedEvent{}, fmt.Errorf("Event contains not enough attributes: %+v", ev)
	}
	if !bytes.Equal(ev.Attributes[0].Key, []byte("BatchIndex")) || !bytes.Equal(ev.Attributes[1].Key, []byte("Privkey")) {
		return PrivkeyGeneratedEvent{}, fmt.Errorf("Bad event attributes: %+v", ev)
	}

	b, err := strconv.Atoi(string(ev.Attributes[0].Value))
	if err != nil {
		return PrivkeyGeneratedEvent{}, err
	}
	privkey, err := app.DecodePrivkeyFromEvent(string(ev.Attributes[1].Value))
	if err != nil {
		return PrivkeyGeneratedEvent{}, err
	}

	return PrivkeyGeneratedEvent{uint64(b), privkey}, nil
}

func MakePubkeyGeneratedEvent(ev abcitypes.Event) (PubkeyGeneratedEvent, error) {
	if len(ev.Attributes) < 2 {
		return PubkeyGeneratedEvent{}, fmt.Errorf("Event contains not enough attributes: %+v", ev)
	}
	if !bytes.Equal(ev.Attributes[0].Key, []byte("BatchIndex")) || !bytes.Equal(ev.Attributes[1].Key, []byte("Pubkey")) {
		return PubkeyGeneratedEvent{}, fmt.Errorf("Bad event attributes: %+v", ev)
	}

	b, err := strconv.Atoi(string(ev.Attributes[0].Value))
	if err != nil {
		return PubkeyGeneratedEvent{}, err
	}
	pubkey, err := app.DecodePubkeyFromEvent(string(ev.Attributes[1].Value))
	if err != nil {
		return PubkeyGeneratedEvent{}, err
	}

	return PubkeyGeneratedEvent{uint64(b), pubkey}, nil
}

func MakeBatchConfigEvent(ev abcitypes.Event) (BatchConfigEvent, error) {
	if len(ev.Attributes) < 3 {
		return BatchConfigEvent{}, fmt.Errorf("Event contains not enough attributes: %+v", ev)
	}
	if !bytes.Equal(ev.Attributes[0].Key, []byte("StartBatchIndex")) ||
		!bytes.Equal(ev.Attributes[1].Key, []byte("Threshhold")) ||
		!bytes.Equal(ev.Attributes[2].Key, []byte("Keypers")) {
		return BatchConfigEvent{}, fmt.Errorf("Bad event attributes: %+v", ev)
	}

	b, err := strconv.Atoi(string(ev.Attributes[0].Value))
	if err != nil {
		return BatchConfigEvent{}, err
	}

	threshhold, err := strconv.Atoi(string(ev.Attributes[1].Value))
	if err != nil {
		return BatchConfigEvent{}, err
	}
	keypers := app.DecodeAddressesFromEvent(string(ev.Attributes[2].Value))
	return BatchConfigEvent{uint64(b), uint32(threshhold), keypers}, nil
}

func MakeEvent(ev abcitypes.Event) (interface{}, error) {
	if ev.Type == "shielder.privkey-generated" {
		res, err := MakePrivkeyGeneratedEvent(ev)
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	if ev.Type == "shielder.pubkey-generated" {
		res, err := MakePubkeyGeneratedEvent(ev)
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	if ev.Type == "shielder.batch-config" {
		res, err := MakeBatchConfigEvent(ev)
		if err != nil {
			return nil, err
		}
		return res, nil

	}
	return nil, fmt.Errorf("Cannot make event from %+v", ev)
}
