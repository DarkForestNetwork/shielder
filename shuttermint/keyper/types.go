package keyper

import (
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tendermint/tendermint/rpc/client"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"

	"shielder/shuttermint/app"
	"shielder/shuttermint/shmsg"
)

// RoundInterval is the duration between the start of two consecutive rounds
var RoundInterval time.Duration = 5 * time.Second

// PrivateKeyDelay is the duration between the start of the public key generation and the the start
// of the private key generation for a single round
var PrivateKeyDelay time.Duration = 45 * time.Second

// BatchParams describes the parameters for single Batch identified by the BatchIndex
type BatchParams struct {
	BatchIndex                    uint64
	BatchConfig                   app.BatchConfig
	KeyperAddress                 common.Address
	PublicKeyGenerationStartTime  time.Time
	PrivateKeyGenerationStartTime time.Time
}

// BatchState is used to manage the key generation process for a single batch inside the keyper
type BatchState struct {
	BatchParams    BatchParams
	MessageSender  *MessageSender
	ContractCaller *ContractCaller
	Events         <-chan IEvent
}

// Keyper is used to run the keyper key generation
type Keyper struct {
	SigningKey          *ecdsa.PrivateKey
	ShielderURL      string
	EthereumURL         string
	mux                 sync.Mutex
	batchIndexToChannel map[uint64]chan IEvent
	txs                 <-chan coretypes.ResultEvent
}

// NewBatchParams creates a new BatchParams struct for the given BatchIndex
func NewBatchParams(batchIndex uint64, keyperAddress common.Address) BatchParams {
	ts := int64(batchIndex) * int64(RoundInterval)

	pubstart := time.Unix(ts/int64(time.Second), ts%int64(time.Second))
	privstart := pubstart.Add(PrivateKeyDelay)
	return BatchParams{
		BatchIndex:                    batchIndex,
		KeyperAddress:                 keyperAddress,
		PublicKeyGenerationStartTime:  pubstart,
		PrivateKeyGenerationStartTime: privstart,
	}
}

// NextBatchIndex computes the BatchIndex for the next batch to be started
func NextBatchIndex(t time.Time) uint64 {
	return uint64((t.UnixNano() + int64(RoundInterval) - 1) / int64(RoundInterval))
}

// MessageSender can be used to sign shmsg.Message's and send them to shuttermint
type MessageSender struct {
	rpcclient  client.Client
	signingKey *ecdsa.PrivateKey
}

// NewMessageSender creates a new MessageSender
func NewMessageSender(cl client.Client, signingKey *ecdsa.PrivateKey) MessageSender {
	return MessageSender{cl, signingKey}
}

// SendMessage signs the given shmsg.Message and sends the message to shuttermint
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
	if res.DeliverTx.Code != 0 {
		return fmt.Errorf("send message: %s", res.DeliverTx.Log)
	}
	return nil
}

// PubkeyGeneratedEvent is generated by shuttermint, when a new public key has been generated.
type PubkeyGeneratedEvent struct {
	BatchIndex uint64
	Pubkey     *ecdsa.PublicKey
}

// PrivkeyGeneratedEvent is generated by shuttermint, when a new private key has been generated
type PrivkeyGeneratedEvent struct {
	BatchIndex uint64
	Privkey    *ecdsa.PrivateKey
}

// BatchConfigEvent is generated by shuttermint, when a new BatchConfg has been added
type BatchConfigEvent struct {
	StartBatchIndex uint64
	Threshold       uint32
	Keypers         []common.Address
}

// EncryptionKeySignatureAddedEvent is generated by shuttermint when a keyper attests to an
// encryption key
type EncryptionKeySignatureAddedEvent struct {
	BatchIndex    uint64
	EncryptionKey []byte
	Signature     []byte
}

// IEvent is an interface for the event types declared above (PubkeyGeneratedEvent,
// PrivkeyGeneratedEvent, BatchConfigEvent, EncryptionkeySignatureAddedEvent)
type IEvent interface {
	IEvent()
}

func (PubkeyGeneratedEvent) IEvent()             {}
func (PrivkeyGeneratedEvent) IEvent()            {}
func (BatchConfigEvent) IEvent()                 {}
func (EncryptionKeySignatureAddedEvent) IEvent() {}

// ContractCaller interacts with the contracts on Ethereum.
type ContractCaller struct {
	ethereumURL                 string
	signingKey                  *ecdsa.PrivateKey
	keyBroadcastContractAddress common.Address
}

// NewContractCaller creates a new ContractCaller.
func NewContractCaller(ethereumURL string, signingKey *ecdsa.PrivateKey, keyBroadcastContractAddress common.Address) ContractCaller {
	return ContractCaller{
		ethereumURL:                 ethereumURL,
		signingKey:                  signingKey,
		keyBroadcastContractAddress: keyBroadcastContractAddress,
	}
}
