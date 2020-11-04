package keyper

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tendermint/tendermint/rpc/client"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/shmsg"
)

// BatchParams describes the parameters for single Batch identified by the BatchIndex
type BatchParams = contract.BatchParams

// BatchState is used to manage the key generation process for a single batch inside the keyper
type BatchState struct {
	BatchParams                   BatchParams
	KeyperConfig                  KeyperConfig
	MessageSender                 *MessageSender
	ContractCaller                *ContractCaller
	pubkeyGenerated               chan PubkeyGeneratedEvent
	privkeyGenerated              chan PrivkeyGeneratedEvent
	encryptionKeySignatureAdded   chan EncryptionKeySignatureAddedEvent
	decryptionSignatureAdded      chan DecryptionSignatureEvent
	cipherExecutionParams         chan CipherExecutionParams
	startBlockSeen                chan struct{}
	endBlockSeen                  chan struct{}
	executionTimeoutBlockSeen     chan struct{}
	startBlockSeenOnce            sync.Once
	endBlockSeenOnce              sync.Once
	executionTimeoutBlockSeenOnce sync.Once
}

// KeyperConfig contains validated configuration parameters for the keyper client
type KeyperConfig struct {
	ShielderURL              string
	EthereumURL                 string
	SigningKey                  *ecdsa.PrivateKey
	ValidatorKey                ed25519.PrivateKey
	ConfigContractAddress       common.Address
	BatcherContractAddress      common.Address
	KeyBroadcastContractAddress common.Address
	ExecutorContractAddress     common.Address
}

// Keyper is used to run the keyper key generation
type Keyper struct {
	sync.Mutex

	Config KeyperConfig
	ethcl  *ethclient.Client
	shmcl  client.Client

	configContract       *contract.ConfigContract
	keyBroadcastContract *contract.KeyBroadcastContract
	batcherContract      *contract.BatcherContract
	executorContract     *contract.ExecutorContract

	batchConfigs          map[uint64]contract.BatchConfig
	batches               map[uint64]*BatchState
	startBlock            *big.Int
	checkedIn             bool
	txs                   <-chan coretypes.ResultEvent
	ctx                   context.Context
	newHeaders            chan *types.Header // start new batches when new block headers arrive
	group                 *errgroup.Group
	ms                    *MessageSender
	executor              Executor
	cipherExecutionParams chan CipherExecutionParams

	dkg *DKGInstance
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
	var tx tmtypes.Tx = tmtypes.Tx(base64.RawURLEncoding.EncodeToString(signedMessage))
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
	Threshold       uint64
	Keypers         []common.Address
}

// EncryptionKeySignatureAddedEvent is generated by shuttermint when a keyper attests to an
// encryption key
type EncryptionKeySignatureAddedEvent struct {
	KeyperIndex   uint64
	BatchIndex    uint64
	EncryptionKey []byte
	Signature     []byte
}

// DecryptionSignatureEvent is generated by shuttermint when a keyper sends a decryption
// signature.
type DecryptionSignatureEvent struct {
	BatchIndex uint64
	Sender     common.Address
	Signature  []byte
}

// NewDKGInstanceEvent is generated by shuttermint when a new DKG process is started.
type NewDKGInstanceEvent struct {
	Eon uint64
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
func (DecryptionSignatureEvent) IEvent()         {}
func (NewDKGInstanceEvent) IEvent()              {}

// ContractCaller interacts with the contracts on Ethereum.
type ContractCaller struct {
	client     *ethclient.Client
	signingKey *ecdsa.PrivateKey

	ConfigContract       *contract.ConfigContract
	KeyBroadcastContract *contract.KeyBroadcastContract
	BatcherContract      *contract.BatcherContract
	ExecutorContract     *contract.ExecutorContract
}

// NewContractCaller creates a new ContractCaller.
func NewContractCaller(
	client *ethclient.Client,
	signingKey *ecdsa.PrivateKey,
	configContract *contract.ConfigContract,
	keyBroadcastContract *contract.KeyBroadcastContract,
	batcherContract *contract.BatcherContract,
	executorContract *contract.ExecutorContract,
) ContractCaller {
	return ContractCaller{
		client:     client,
		signingKey: signingKey,

		ConfigContract:       configContract,
		KeyBroadcastContract: keyBroadcastContract,
		BatcherContract:      batcherContract,
		ExecutorContract:     executorContract,
	}
}

// Executor is responsible for making sure batches are executed.
type Executor struct {
	ctx                   context.Context
	client                *ethclient.Client
	cc                    *ContractCaller
	cipherExecutionParams <-chan CipherExecutionParams
}

// CipherExecutionParams is the set of parameters necessary to execute a batch.
type CipherExecutionParams struct {
	BatchIndex              uint64
	CipherBatchHash         common.Hash
	DecryptionKey           *ecdsa.PrivateKey
	DecryptedTxs            [][]byte
	DecryptionSignerIndices []uint64
	DecryptionSignatures    [][]byte
}
