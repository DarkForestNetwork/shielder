package app

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"

	"shielder/shuttermint/shmsg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tendermint/tendermint/abci/types"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/kv"
)

// Visit https://github.com/tendermint/spec/blob/master/spec/abci/abci.md for more information on
// the application interface we're implementing here.
// https://docs.tendermint.com/master/spec/abci/apps.html also provides some useful information

// CheckTx checks if a transaction is valid. If return Code != 0, it will be rejected from the
// mempool and hence not broadcasted to other peers and not included in a proposal block.
func (app *ShielderApp) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	return abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}
}

// NewShielderApp creates a new ShielderApp
func NewShielderApp() *ShielderApp {
	return &ShielderApp{
		Configs: []*BatchConfig{{}},
		Batches: make(map[uint64]BatchKeys),
	}
}

// getConfig returns the BatchConfig for the given batchIndex
func (app *ShielderApp) getConfig(batchIndex uint64) *BatchConfig {
	for i := len(app.Configs) - 1; i >= 0; i-- {
		if app.Configs[i].StartBatchIndex <= batchIndex {
			return app.Configs[i]
		}
	}
	panic("guard element missing")
}

func (app *ShielderApp) addConfig(cfg BatchConfig) error {
	lastConfig := app.Configs[len(app.Configs)-1]
	if lastConfig.StartBatchIndex >= cfg.StartBatchIndex {
		return errors.New("StartBatchIndex must be greater than previous StartBatchIndex")
	}
	app.Configs = append(app.Configs, &cfg)
	return nil
}

// getBatch returns the BatchKeys for the given batchIndex
func (app *ShielderApp) getBatch(batchIndex uint64) BatchKeys {
	bk, ok := app.Batches[batchIndex]
	if !ok {
		bk.Config = app.getConfig(batchIndex)
	}

	return bk
}

func (ShielderApp) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{}
}

func (ShielderApp) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return abcitypes.ResponseSetOption{}
}

/* BlockExecution

The first time a new blockchain is started, Tendermint calls InitChain. From then on, the following
sequence of methods is executed for each block:

BeginBlock, [DeliverTx], EndBlock, Commit

where one DeliverTx is called for each transaction in the block. The result is an updated
application state. Cryptographic commitments to the results of DeliverTx, EndBlock, and Commit are
included in the header of the next block.
*/
func (ShielderApp) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	return abcitypes.ResponseInitChain{}
}

func (ShielderApp) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	// fmt.Println("BEGIN", req.GetHash(), req.Header.GetChainID(), req.Header.GetTime())
	return abcitypes.ResponseBeginBlock{}
}

// decodeTx decodes the given transaction.  It's kind of strange that we have do URL decode the
// message outselves instead of tendermint doing it for us.
func (ShielderApp) decodeTx(req abcitypes.RequestDeliverTx) (signer common.Address, msg *shmsg.Message, err error) {
	var signedMsg []byte
	signedMsg, err = base64.RawURLEncoding.DecodeString(string(req.Tx))
	if err != nil {
		return
	}
	signer, err = shmsg.GetSigner(signedMsg)
	if err != nil {
		return
	}

	msg, err = shmsg.GetMessage(signedMsg)

	if err != nil {
		return
	}
	return
}

func (app *ShielderApp) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {
	// signedTransaction := make([]byte,  base64.RawURLEncoding.DecodedLen(len(req.Tx)))

	signer, msg, err := app.decodeTx(req)

	if err != nil {
		fmt.Println("Error while decoding transaction:", err)
		return abcitypes.ResponseDeliverTx{
			Code:   1,
			Events: []types.Event{}}
	}
	return app.deliverMessage(msg, signer)
}

// encodePubkeyForEvent encodes the PublicKey as a string suitable for putting it into a tendermint
// event, i.e. an utf-8 compatible string
func encodePubkeyForEvent(pubkey *ecdsa.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(crypto.FromECDSAPub(pubkey))
}

func (app *ShielderApp) deliverPublicKeyCommitment(pkc *shmsg.PublicKeyCommitment, sender common.Address) abcitypes.ResponseDeliverTx {
	bk := app.getBatch(pkc.BatchIndex)
	publicKeyBefore := bk.PublicKey
	err := bk.AddPublicKeyCommitment(PublicKeyCommitment{Sender: sender, Pubkey: pkc.Commitment})
	if err != nil {
		fmt.Println("GOT ERROR", err)
		return abcitypes.ResponseDeliverTx{
			Code:   1,
			Events: []types.Event{}}

	}
	app.Batches[pkc.BatchIndex] = bk

	var events []types.Event
	if publicKeyBefore == nil && bk.PublicKey != nil {
		// we have generated a public key with this PublicKeyCommitment
		events = append(events, types.Event{
			Type: "shielder.pubkey-generated",
			Attributes: []kv.Pair{
				{Key: []byte("BatchIndex"), Value: []byte(fmt.Sprintf("%d", pkc.BatchIndex))},
				{Key: []byte("Pubkey"), Value: []byte(encodePubkeyForEvent(bk.PublicKey))}},
		})
	}
	return abcitypes.ResponseDeliverTx{
		Code:   0,
		Events: events}
}

func (app *ShielderApp) deliverMessage(msg *shmsg.Message, sender common.Address) abcitypes.ResponseDeliverTx {
	fmt.Println("MSG:", msg)
	if msg.GetPublicKeyCommitment() != nil {
		return app.deliverPublicKeyCommitment(msg.GetPublicKeyCommitment(), sender)
	}
	return abcitypes.ResponseDeliverTx{
		Code:   0,
		Events: []types.Event{}}
}

func (ShielderApp) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	// fmt.Println("END BLOCK", req.Height)
	return abcitypes.ResponseEndBlock{}
}

func (app *ShielderApp) Commit() abcitypes.ResponseCommit {
	// fmt.Printf("COMMIT %#v", app.games)
	return abcitypes.ResponseCommit{}
}

func (ShielderApp) Query(req abcitypes.RequestQuery) abcitypes.ResponseQuery {
	return abcitypes.ResponseQuery{Code: 0}
}
