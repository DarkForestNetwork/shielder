package shielderevents_test

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/stretchr/testify/require"

	"shielder/shuttermint/crypto"
	"shielder/shuttermint/keyper/shielderevents"
)

var (
	polynomial *crypto.Polynomial
	gammas     crypto.Gammas
	eon        = uint64(64738)
	sender     = common.BytesToAddress([]byte("foo"))
	addresses  = []common.Address{
		common.BigToAddress(big.NewInt(1)),
		common.BigToAddress(big.NewInt(2)),
		common.BigToAddress(big.NewInt(3)),
	}
)

func init() {
	var err error
	polynomial, err = crypto.RandomPolynomial(rand.Reader, 3)
	if err != nil {
		panic(err)
	}

	gammas = *polynomial.Gammas()
}

// roundtrip checks that the given IEvent round-trips, i.e. it can be serialized as an ABCI Event
// and deserialized back again to an equal value.
func roundtrip(t *testing.T, ev shielderevents.IEvent) {
	ev2, err := shielderevents.MakeEvent(ev.MakeABCIEvent(), 0)
	require.Nil(t, err)
	require.Equal(t, ev, ev2)
}

func TestAccusation(t *testing.T) {
	ev := shielderevents.Accusation{
		Eon:     eon,
		Sender:  sender,
		Accused: addresses,
	}
	roundtrip(t, ev)
}

func TestEmptyAccusation(t *testing.T) {
	ev := shielderevents.Accusation{
		Eon:    eon,
		Sender: sender,
	}
	roundtrip(t, ev)
}

func TestApology(t *testing.T) {
	accusers := addresses
	var polyEval []*big.Int
	for i := 0; i < len(accusers); i++ {
		eval := big.NewInt(int64(100 + i))
		polyEval = append(polyEval, eval)
	}
	ev := shielderevents.Apology{
		Eon:      eon,
		Sender:   sender,
		Accusers: accusers,
		PolyEval: polyEval,
	}
	roundtrip(t, ev)
}

func TestEmptyApology(t *testing.T) {
	ev := shielderevents.Apology{
		Eon:    eon,
		Sender: sender,
	}
	roundtrip(t, ev)
}

func TestBatchConfig(t *testing.T) {
	ev := shielderevents.BatchConfig{
		StartBatchIndex: 111,
		Threshold:       2,
		Keypers:         addresses,
		ConfigIndex:     uint64(0xffffffffffffffff),
	}
	roundtrip(t, ev)
}

func TestCheckIn(t *testing.T) {
	privateKeyECDSA, err := ethcrypto.GenerateKey()
	require.Nil(t, err)
	publicKey := ecies.ImportECDSAPublic(&privateKeyECDSA.PublicKey)
	ev := shielderevents.CheckIn{Sender: sender, EncryptionPublicKey: publicKey}
	roundtrip(t, ev)
}

func TestDecryptionSignature(t *testing.T) {
	ev := shielderevents.DecryptionSignature{
		BatchIndex: uint64(64738),
		Sender:     sender,
		Signature:  []byte("fooobar"),
	}
	roundtrip(t, ev)
}

func TestEonStarted(t *testing.T) {
	ev := shielderevents.EonStarted{Eon: eon, BatchIndex: 9999}
	roundtrip(t, ev)
}

func TestPolyCommitment(t *testing.T) {
	ev := shielderevents.PolyCommitment{
		Eon:    eon,
		Sender: sender,
		Gammas: &gammas,
	}
	roundtrip(t, ev)
}

func TestPolyEval(t *testing.T) {
	var receivers []common.Address
	var encryptedEvals [][]byte
	for i := 1; i < 10; i++ {
		receivers = append(receivers, common.BigToAddress(new(big.Int).SetUint64(uint64(i))))
		encryptedEvals = append(encryptedEvals, []byte(fmt.Sprintf("encrypted: %d", i)))
	}
	ev := shielderevents.PolyEval{
		Eon:            eon,
		Sender:         sender,
		Receivers:      receivers,
		EncryptedEvals: encryptedEvals,
	}
	roundtrip(t, ev)
}
