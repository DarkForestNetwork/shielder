package app

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"shielder/shuttermint/app/evtype"
)

func TestEvents(t *testing.T) {
	eon := uint64(5)
	sender := common.BigToAddress(new(big.Int).SetUint64(123))
	anotherAddress := common.BigToAddress(new(big.Int).SetUint64(456))
	data := []byte("some data")

	t.Run("MakePolyEvalRegisteredEvent", func(t *testing.T) {
		msg := &PolyEval{
			Sender:         sender,
			Eon:            eon,
			Receivers:      []common.Address{anotherAddress},
			EncryptedEvals: [][]byte{data},
		}
		ev := MakePolyEvalRegisteredEvent(msg)
		require.Equal(t, evtype.PolyEval, ev.Type)
		require.Equal(t, 4, len(ev.Attributes))
		require.Equal(t, []byte("Sender"), ev.Attributes[0].Key)
		require.Equal(t, []byte(sender.Hex()), ev.Attributes[0].Value)
		require.Equal(t, []byte("Eon"), ev.Attributes[1].Key)
		require.Equal(t, []byte("5"), ev.Attributes[1].Value)
		require.Equal(t, []byte("Receivers"), ev.Attributes[2].Key)
		require.Equal(t, []byte(anotherAddress.Hex()), ev.Attributes[2].Value)
		require.Equal(t, []byte("EncryptedEvals"), ev.Attributes[3].Key)
		require.Equal(t, []byte(hexutil.Encode(data)), ev.Attributes[3].Value)
	})

	t.Run("MakePolyCommitmentRegisteredEvent", func(t *testing.T) {
		msg := &PolyCommitment{
			Sender: sender,
			Eon:    eon,
		}
		ev := MakePolyCommitmentRegisteredEvent(msg)
		require.Equal(t, evtype.PolyCommitment, ev.Type)
		require.Equal(t, 3, len(ev.Attributes))
		require.Equal(t, []byte("Sender"), ev.Attributes[0].Key)
		require.Equal(t, []byte(sender.Hex()), ev.Attributes[0].Value)
		require.Equal(t, []byte("Eon"), ev.Attributes[1].Key)
		require.Equal(t, []byte("5"), ev.Attributes[1].Value)
		require.Equal(t, []byte("Gammas"), ev.Attributes[2].Key)
	})

	t.Run("MakeAccusationRegisteredEvent", func(t *testing.T) {
		msg := &Accusation{
			Sender:  sender,
			Eon:     eon,
			Accused: []common.Address{anotherAddress},
		}
		ev := MakeAccusationRegisteredEvent(msg)
		require.Equal(t, evtype.Accusation, ev.Type)
		require.Equal(t, 3, len(ev.Attributes))
		require.Equal(t, []byte("Sender"), ev.Attributes[0].Key)
		require.Equal(t, []byte(sender.Hex()), ev.Attributes[0].Value)
		require.Equal(t, []byte("Eon"), ev.Attributes[1].Key)
		require.Equal(t, []byte("5"), ev.Attributes[1].Value)
		require.Equal(t, []byte("Accused"), ev.Attributes[2].Key)
		require.Equal(t, []byte(anotherAddress.Hex()), ev.Attributes[2].Value)
	})
}
