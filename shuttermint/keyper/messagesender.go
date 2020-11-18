package keyper

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"

	"github.com/tendermint/tendermint/rpc/client"
	tmtypes "github.com/tendermint/tendermint/types"

	"shielder/shuttermint/shmsg"
)

// NewRPCMessageSender creates a new RPCMessageSender
func NewRPCMessageSender(cl client.Client, signingKey *ecdsa.PrivateKey) RPCMessageSender {
	return RPCMessageSender{cl, signingKey}
}

// SendMessage signs the given shmsg.Message and sends the message to shuttermint
func (ms RPCMessageSender) SendMessage(ctx context.Context, msg *shmsg.Message) error {
	signedMessage, err := shmsg.SignMessage(msg, ms.signingKey)
	if err != nil {
		return err
	}
	var tx tmtypes.Tx = tmtypes.Tx(base64.RawURLEncoding.EncodeToString(signedMessage))
	res, err := ms.rpcclient.BroadcastTxCommit(ctx, tx)
	if err != nil {
		return err
	}
	if res.DeliverTx.Code != 0 {
		return fmt.Errorf("remote error: %s", res.DeliverTx.Log)
	}
	return nil
}

// NewMockMessageSender creates a new MockMessageSender
func NewMockMessageSender() MockMessageSender {
	return MockMessageSender{
		Msgs: make(chan *shmsg.Message),
	}
}

func (ms MockMessageSender) SendMessage(_ context.Context, msg *shmsg.Message) error {
	ms.Msgs <- msg
	return nil
}
