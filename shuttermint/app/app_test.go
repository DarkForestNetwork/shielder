package app

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"testing"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"shielder/shuttermint/keyper/shielderevents"
	"shielder/shuttermint/shmsg"
)

func TestNewShielderApp(t *testing.T) {
	app := NewShielderApp()
	require.Equal(t, len(app.Configs), 1, "Configs should contain exactly one guard element")
	require.Equal(t, app.Configs[0], &BatchConfig{}, "Bad guard element")
}

func TestGetBatch(t *testing.T) {
	app := NewShielderApp()

	err := app.addConfig(BatchConfig{
		ConfigIndex:     1,
		StartBatchIndex: 100,
		Threshold:       1,
		Keypers:         addr,
	})
	require.Nil(t, err)

	err = app.addConfig(BatchConfig{
		ConfigIndex:     2,
		StartBatchIndex: 200,
		Threshold:       2,
		Keypers:         addr,
	})
	require.Nil(t, err)

	err = app.addConfig(BatchConfig{
		ConfigIndex:     3,
		StartBatchIndex: 300,
		Threshold:       3,
		Keypers:         addr,
	})
	require.Nil(t, err)

	require.Equal(t, uint64(0), app.getBatchState(0).Config.Threshold)
	require.Equal(t, uint64(0), app.getBatchState(99).Config.Threshold)
	require.Equal(t, uint64(1), app.getBatchState(100).Config.Threshold)
	require.Equal(t, uint64(1), app.getBatchState(101).Config.Threshold)
	require.Equal(t, uint64(1), app.getBatchState(199).Config.Threshold)
	require.Equal(t, uint64(2), app.getBatchState(200).Config.Threshold)
	require.Equal(t, uint64(3), app.getBatchState(1000).Config.Threshold)
}

func TestAddConfig(t *testing.T) {
	app := NewShielderApp()

	err := app.addConfig(BatchConfig{
		ConfigIndex:     1,
		StartBatchIndex: 100,
		Threshold:       1,
		Keypers:         addr,
	})
	require.Nil(t, err)

	err = app.addConfig(BatchConfig{
		ConfigIndex:     2,
		StartBatchIndex: 99,
		Threshold:       1,
		Keypers:         addr,
	})
	require.NotNil(t, err, "Expected error, StartBatchIndex must not decrease")

	err = app.addConfig(BatchConfig{
		ConfigIndex:     1,
		StartBatchIndex: 100,
		Threshold:       1,
		Keypers:         addr,
	})
	require.NotNil(t, err, "Expected error, ConfigIndex must increase")

	err = app.addConfig(BatchConfig{
		ConfigIndex:     2,
		StartBatchIndex: 100,
		Threshold:       2,
		Keypers:         addr,
	})
	require.Nil(t, err)
}

func TestEncodePubkeyForEvent(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.Nil(t, err, "Could not generate key")
	encoded := encodePubkeyForEvent(&key.PublicKey)
	t.Logf("Encoded: %s", encoded)
	require.True(t, utf8.ValidString(encoded))

	decoded, err := shielderevents.DecodePubkey(encoded)
	require.Nil(t, err, "could not decode pubkey")
	t.Logf("Decoded: %+v", decoded)
	require.Equal(t, key.PublicKey, *decoded)
}

func TestAddDecryptionSignature(t *testing.T) {
	app := NewShielderApp()
	keypers := addresses[:3]
	err := app.addConfig(BatchConfig{
		ConfigIndex:     1,
		StartBatchIndex: 100,
		Threshold:       2,
		Keypers:         keypers,
	})
	require.Nil(t, err)

	// don't accept signature from non-keyper
	res1 := app.deliverDecryptionSignature(
		&shmsg.DecryptionSignature{
			BatchIndex: 200,
			Signature:  []byte("signature"),
		},
		addresses[3],
	)
	require.True(t, res1.IsErr())
	require.Empty(t, res1.Events)

	// accept signature from keyper
	res2 := app.deliverDecryptionSignature(
		&shmsg.DecryptionSignature{
			BatchIndex: 200,
			Signature:  []byte("signature"),
		},
		keypers[0],
	)
	require.True(t, res2.IsOK())
	require.Equal(t, 1, len(res2.Events))

	ev := res2.Events[0]
	require.Equal(t, "shielder.decryption-signature", ev.Type)
	require.Equal(t, []byte("BatchIndex"), ev.Attributes[0].Key)
	require.Equal(t, []byte("200"), ev.Attributes[0].Value)
	require.Equal(t, []byte("Sender"), ev.Attributes[1].Key)
	require.Equal(t, []byte(keypers[0].Hex()), ev.Attributes[1].Value)
	require.Equal(t, []byte("Signature"), ev.Attributes[2].Key)
	decodedSignature, _ := base64.RawURLEncoding.DecodeString(string(ev.Attributes[2].Value))
	require.Equal(t, []byte("signature"), decodedSignature)

	// don't accept another signature
	res3 := app.deliverDecryptionSignature(
		&shmsg.DecryptionSignature{
			BatchIndex: 200,
			Signature:  []byte("signature"),
		},
		keypers[0],
	)
	require.True(t, res3.IsErr())
	require.Empty(t, res1.Events)
}

func ensureGobable(t *testing.T, obj interface{}) {
	buff := bytes.Buffer{}
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(obj)
	require.Nil(t, err)
}

func TestGobDKG(t *testing.T) {
	var eon uint64 = 201
	var err error
	keypers := addr
	dkg := NewDKGInstance(BatchConfig{
		ConfigIndex:     1,
		StartBatchIndex: 100,
		Threshold:       1,
		Keypers:         keypers,
	}, eon)

	err = dkg.RegisterAccusationMsg(AccusationMsg{
		Sender:  keypers[0],
		Eon:     eon,
		Accused: []common.Address{keypers[1]},
	})
	require.Nil(t, err)

	err = dkg.RegisterApologyMsg(ApologyMsg{
		Sender:   keypers[0],
		Eon:      eon,
		Accusers: []common.Address{keypers[1]},
	})
	require.Nil(t, err)

	err = dkg.RegisterPolyCommitmentMsg(PolyCommitmentMsg{
		Sender: keypers[0],
		Eon:    eon,
	})
	require.Nil(t, err)

	err = dkg.RegisterPolyEvalMsg(PolyEval{
		Sender:         keypers[0],
		Eon:            eon,
		Receivers:      []common.Address{keypers[1]},
		EncryptedEvals: [][]byte{[]byte{}},
	})
	require.Nil(t, err)

	ensureGobable(t, dkg)
}
