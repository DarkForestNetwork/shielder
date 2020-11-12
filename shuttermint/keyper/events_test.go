package keyper

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/ecies"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"shielder/shuttermint/app"
)

func TestCheckInEvent(t *testing.T) {
	sender := common.BigToAddress(big.NewInt(1))
	privateKeyECDSA, err := crypto.GenerateKey()
	publicKey := ecies.ImportECDSAPublic(&privateKeyECDSA.PublicKey)
	require.Nil(t, err)
	appEv := app.MakeCheckInEvent(sender, publicKey)
	evInt, err := MakeEvent(appEv)
	require.Nil(t, err)
	ev, ok := evInt.(CheckInEvent)
	require.True(t, ok)
	require.Equal(t, sender, ev.Sender)
	require.True(t, ev.EncryptionPublicKey.ExportECDSA().Equal(&privateKeyECDSA.PublicKey))
}

func TestMakeEventBatchConfig(t *testing.T) {
	var addresses []common.Address = []common.Address{
		common.BigToAddress(big.NewInt(1)),
		common.BigToAddress(big.NewInt(2)),
		common.BigToAddress(big.NewInt(3)),
	}

	appEvent := app.MakeBatchConfigEvent(111, 2, addresses)
	ev, err := MakeEvent(appEvent)
	require.Nil(t, err)
	require.Equal(t,
		BatchConfigEvent{
			StartBatchIndex: 111,
			Threshold:       2,
			Keypers:         addresses,
		},
		ev)
}

func TestMakeEventEncryptionSignatureAddedEvent(t *testing.T) {
	var keyperIndex uint64 = 3
	var batchIndex uint64 = 111
	key := []byte("key")
	sig := []byte("sig")
	appEvent := app.MakeEncryptionKeySignatureAddedEvent(keyperIndex, batchIndex, key, sig)
	ev, err := MakeEvent(appEvent)
	expectedEvent := EncryptionKeySignatureAddedEvent{
		KeyperIndex:   keyperIndex,
		BatchIndex:    batchIndex,
		EncryptionKey: key,
		Signature:     sig,
	}
	require.Nil(t, err)
	require.Equal(t, ev, expectedEvent)
}

func TestMakeNewDKGInstanceEvent(t *testing.T) {
	var eon uint64 = 10
	var configIndex uint64 = 20
	appEv := app.MakeNewDKGInstanceEvent(eon, configIndex)
	ev, err := MakeEvent(appEv)
	expectedEv := NewDKGInstanceEvent{
		Eon:         eon,
		ConfigIndex: configIndex,
	}
	require.Nil(t, err)
	require.Equal(t, expectedEv, ev)
}
