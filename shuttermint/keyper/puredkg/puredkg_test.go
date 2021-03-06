package puredkg

import (
	"crypto/rand"
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"shielder/shuttermint/shcrypto"
)

func TestPureDKGFull(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(3)
	threshold := uint64(2)

	dkgs := []*PureDKG{}
	for i := uint64(0); i < numKeypers; i++ {
		dkg := NewPureDKG(eon, numKeypers, threshold, i)
		dkgs = append(dkgs, &dkg)
	}

	// dealing phase
	for _, dkg := range dkgs {
		polyCommitmentMsg, polyEvalMsgs, err := dkg.StartPhase1Dealing()
		require.Nil(t, err)

		for _, receiverDKG := range dkgs {
			err := receiverDKG.HandlePolyCommitmentMsg(polyCommitmentMsg)
			require.Nil(t, err)
		}

		require.Equal(t, int(numKeypers), 1+len(polyEvalMsgs))
		for _, msg := range polyEvalMsgs {
			err := dkgs[msg.Receiver].HandlePolyEvalMsg(msg)
			require.Nil(t, err)
		}
	}

	// accusation phase
	for _, dkg := range dkgs {
		accusations := dkg.StartPhase2Accusing()
		require.Zero(t, len(accusations))
	}

	// apology phase
	for _, dkg := range dkgs {
		apologies := dkg.StartPhase3Apologizing()
		require.Zero(t, len(apologies))
	}

	// finalize
	for _, dkg := range dkgs {
		dkg.Finalize()
	}

	var results []Result

	for _, dkg := range dkgs {
		result, err := dkg.ComputeResult()
		require.Nil(t, err)
		results = append(results, result)
	}
	for _, r := range results {
		require.True(t, reflect.DeepEqual(r.PublicKey, results[0].PublicKey))
	}
}

// TestPureDKGOfflineSendAccusation tests that we send accusations when we don't receive any
// message from a keyper. See https://shielder/issues/62
func TestPureDKGOfflineSendAccusation(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(3)
	threshold := uint64(2)

	dkg := NewPureDKG(eon, numKeypers, threshold, 0)

	// dealing phase
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	accusations := dkg.StartPhase2Accusing()
	require.Equal(t, 2, len(accusations))
}

func TestPureDKGCorrupt(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(3)
	threshold := uint64(2)

	dkgs := []*PureDKG{}
	for i := uint64(0); i < numKeypers; i++ {
		dkg := NewPureDKG(eon, numKeypers, threshold, i)
		dkgs = append(dkgs, &dkg)
	}
	honestDKGs := dkgs[:len(dkgs)-1]
	corruptDKG := dkgs[len(dkgs)-1]

	// dealing phase
	for _, dkg := range honestDKGs {
		polyCommitmentMsg, polyEvalMsgs, err := dkg.StartPhase1Dealing()
		require.Nil(t, err)

		for _, receiverDKG := range dkgs {
			require.Nil(t, receiverDKG.HandlePolyCommitmentMsg(polyCommitmentMsg))
		}
		for _, msg := range polyEvalMsgs {
			require.Nil(t, dkgs[msg.Receiver].HandlePolyEvalMsg(msg))
		}
	}

	// corrupt DKG sends invalid poly eval to first keyper
	polyCommitmentMsg, polyEvalMsgs, err := corruptDKG.StartPhase1Dealing()
	require.Nil(t, err)
	for _, receiverDKG := range dkgs {
		require.Nil(t, receiverDKG.HandlePolyCommitmentMsg(polyCommitmentMsg))
	}
	for _, msg := range polyEvalMsgs {
		if msg.Receiver == 0 {
			corruptMsg := PolyEvalMsg{
				Eon:      msg.Eon,
				Sender:   msg.Sender,
				Receiver: msg.Receiver,
				Eval:     big.NewInt(666),
			}
			require.Nil(t, dkgs[msg.Receiver].HandlePolyEvalMsg(corruptMsg))
		} else {
			require.Nil(t, dkgs[msg.Receiver].HandlePolyEvalMsg(msg))
		}
	}

	// accusation phase
	for _, dkg := range dkgs[1:] {
		accusations := dkg.StartPhase2Accusing()
		require.Zero(t, len(accusations))
	}
	accusations := dkgs[0].StartPhase2Accusing()
	require.Equal(t, 1, len(accusations))
	for _, dkg := range dkgs {
		err := dkg.HandleAccusationMsg(accusations[0])
		require.Nil(t, err)
	}

	// apology phase
	for _, dkg := range honestDKGs {
		apologies := dkg.StartPhase3Apologizing()
		require.Zero(t, len(apologies))
	}
	apologies := corruptDKG.StartPhase3Apologizing()
	require.Equal(t, 1, len(apologies))
	for _, dkg := range dkgs {
		msg := ApologyMsg{
			Eon:     apologies[0].Eon,
			Accuser: apologies[0].Accuser,
			Accused: apologies[0].Accused,
			Eval:    big.NewInt(121212),
		}
		err := dkg.HandleApologyMsg(msg)
		require.Nil(t, err)
	}

	// finalize
	for _, dkg := range dkgs {
		dkg.Finalize()
	}

	var results []Result
	for _, dkg := range dkgs {
		result, err := dkg.ComputeResult()
		require.Nil(t, err)
		results = append(results, result)
	}
	for _, r := range results {
		require.True(t, reflect.DeepEqual(r.PublicKey, results[0].PublicKey))
	}
}

func TestDealingSendsCorrectMsgs(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(3)
	threshold := uint64(2)
	keyper := uint64(1)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	commitmentMsg, evalMsgs, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	require.Equal(t, eon, commitmentMsg.Eon)
	require.Equal(t, keyper, commitmentMsg.Sender)
	expectedGammas := dkg.Polynomial.Gammas()
	require.Equal(t, len(*expectedGammas), len(*commitmentMsg.Gammas))
	for i, g := range *commitmentMsg.Gammas {
		require.True(t, shcrypto.EqualG2(g, (*expectedGammas)[i]))
	}

	require.Equal(t, int(numKeypers), 1+len(evalMsgs))
	for _, msg := range evalMsgs {
		require.Equal(t, eon, msg.Eon)
		require.Equal(t, keyper, msg.Sender)
		require.Equal(t, dkg.Polynomial.EvalForKeyper(int(msg.Receiver)), msg.Eval)
	}
}

func TestAccusing(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(4)
	threshold := uint64(3)
	keyper := uint64(3)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	polys := []*shcrypto.Polynomial{}
	for i := 0; i < int(numKeypers); i++ {
		p, err := shcrypto.RandomPolynomial(rand.Reader, shcrypto.DegreeFromThreshold(threshold))
		require.Nil(t, err)
		polys = append(polys, p)
	}

	makeCommitmentMsg := func(sender int) PolyCommitmentMsg {
		return PolyCommitmentMsg{
			Eon:    eon,
			Sender: uint64(sender),
			Gammas: polys[sender].Gammas(),
		}
	}
	makeEvalMsg := func(sender int) PolyEvalMsg {
		return PolyEvalMsg{
			Eon:      eon,
			Sender:   uint64(sender),
			Receiver: keyper,
			Eval:     polys[sender].EvalForKeyper(int(keyper)),
		}
	}

	// first keyper is honest
	commitmentMsg0 := makeCommitmentMsg(0)
	evalMsg0 := makeEvalMsg(0)
	err = dkg.HandlePolyCommitmentMsg(commitmentMsg0)
	require.Nil(t, err)
	err = dkg.HandlePolyEvalMsg(evalMsg0)
	require.Nil(t, err)

	// second keyper is dishonest
	commitmentMsg1 := makeCommitmentMsg(1)
	evalMsg1 := makeEvalMsg(1)
	evalMsg1.Eval = big.NewInt(123)
	err = dkg.HandlePolyCommitmentMsg(commitmentMsg1)
	require.Nil(t, err)
	err = dkg.HandlePolyEvalMsg(evalMsg1)
	require.Nil(t, err)

	// third keyper misses eval
	commitmentMsg2 := makeCommitmentMsg(2)
	err = dkg.HandlePolyCommitmentMsg(commitmentMsg2)
	require.Nil(t, err)
	accusations := dkg.StartPhase2Accusing()
	require.Equal(t, 2, len(accusations))
	accDishonest := accusations[0]
	accMissed := accusations[1]

	require.Equal(t, eon, accDishonest.Eon)
	require.Equal(t, keyper, accDishonest.Accuser)
	require.Equal(t, KeyperIndex(1), accDishonest.Accused)

	require.Equal(t, eon, accMissed.Eon)
	require.Equal(t, keyper, accMissed.Accuser)
	require.Equal(t, KeyperIndex(2), accMissed.Accused)
}

func TestApologizing(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(4)
	threshold := uint64(3)
	keyper := uint64(3)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	_ = dkg.StartPhase2Accusing()

	accusation := AccusationMsg{
		Eon:     eon,
		Accuser: uint64(1),
		Accused: keyper,
	}
	err = dkg.HandleAccusationMsg(accusation)
	require.Nil(t, err)

	apologies := dkg.StartPhase3Apologizing()
	require.Equal(t, 1, len(apologies))
	require.Equal(t, eon, apologies[0].Eon)
	require.Equal(t, keyper, apologies[0].Accused)
	require.Equal(t, accusation.Accuser, apologies[0].Accuser)
	require.Equal(t, dkg.Polynomial.EvalForKeyper(int(accusation.Accuser)), apologies[0].Eval)
}

func TestInvalidCommitmentHandling(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(4)
	threshold := uint64(3)
	keyper := uint64(3)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	makeCommitmentMsg := func() PolyCommitmentMsg {
		p, err := shcrypto.RandomPolynomial(rand.Reader, shcrypto.DegreeFromThreshold(threshold))
		require.Nil(t, err)
		return PolyCommitmentMsg{
			Eon:    eon,
			Sender: uint64(1),
			Gammas: p.Gammas(),
		}
	}

	c1 := makeCommitmentMsg()
	c1.Eon = uint64(10)
	require.NotNil(t, dkg.HandlePolyCommitmentMsg(c1))

	c2 := makeCommitmentMsg()
	gammas := (*c2.Gammas)[:len(*c2.Gammas)-1]
	c2.Gammas = &gammas
	require.NotNil(t, dkg.HandlePolyCommitmentMsg(c2))

	c3 := makeCommitmentMsg()
	require.Nil(t, dkg.HandlePolyCommitmentMsg(c3))
	require.NotNil(t, dkg.HandlePolyCommitmentMsg(c3))
}

func TestInvalidEvalHandling(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(4)
	threshold := uint64(3)
	keyper := uint64(3)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	makeEvalMsg := func() PolyEvalMsg {
		p, err := shcrypto.RandomPolynomial(rand.Reader, shcrypto.DegreeFromThreshold(threshold))
		require.Nil(t, err)
		return PolyEvalMsg{
			Eon:      eon,
			Sender:   uint64(1),
			Receiver: keyper,
			Eval:     p.EvalForKeyper(int(keyper)),
		}
	}

	m1 := makeEvalMsg()
	m1.Eon = uint64(10)
	require.NotNil(t, dkg.HandlePolyEvalMsg(m1))

	m2 := makeEvalMsg()
	m2.Receiver = uint64(0)
	require.NotNil(t, dkg.HandlePolyEvalMsg(m2))

	m3 := makeEvalMsg()
	m3.Eval = big.NewInt(-1)
	require.NotNil(t, dkg.HandlePolyEvalMsg(m3))

	m4 := makeEvalMsg()
	require.Nil(t, dkg.HandlePolyEvalMsg(m4))
	require.NotNil(t, dkg.HandlePolyEvalMsg(m4))

	dkg.StartPhase2Accusing()
	m5 := makeEvalMsg()
	m5.Sender = uint64(2)
	require.NotNil(t, dkg.HandlePolyEvalMsg(m5))
}

func TestInvalidAccusationHandling(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(4)
	threshold := uint64(3)
	keyper := uint64(3)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	makeAccusationMsg := func() AccusationMsg {
		return AccusationMsg{
			Eon:     eon,
			Accuser: uint64(1),
			Accused: keyper,
		}
	}

	m1 := makeAccusationMsg()
	m1.Eon = uint64(10)
	require.NotNil(t, dkg.HandleAccusationMsg(m1))

	m2 := makeAccusationMsg()
	require.Nil(t, dkg.HandleAccusationMsg(m2))
	require.NotNil(t, dkg.HandleAccusationMsg(m2))

	dkg.StartPhase2Accusing()
	dkg.StartPhase3Apologizing()

	m3 := makeAccusationMsg()
	m3.Accuser = uint64(0)
	require.NotNil(t, dkg.HandleAccusationMsg(m3))
}

func TestInvalidApologyHandling(t *testing.T) {
	eon := uint64(5)
	numKeypers := uint64(4)
	threshold := uint64(3)
	keyper := uint64(3)
	dkg := NewPureDKG(eon, numKeypers, threshold, keyper)
	_, _, err := dkg.StartPhase1Dealing()
	require.Nil(t, err)

	makeApologyMsg := func() ApologyMsg {
		return ApologyMsg{
			Eon:     eon,
			Accuser: uint64(1),
			Accused: keyper,
			Eval:    big.NewInt(1),
		}
	}

	m1 := makeApologyMsg()
	m1.Eon = uint64(10)
	require.NotNil(t, dkg.HandleApologyMsg(m1))

	m2 := makeApologyMsg()
	m2.Eval = big.NewInt(-1)
	require.NotNil(t, dkg.HandleApologyMsg(m2))

	m3 := makeApologyMsg()
	require.Nil(t, dkg.HandleApologyMsg(m3))
	require.NotNil(t, dkg.HandleApologyMsg(m3))

	dkg.StartPhase2Accusing()
	dkg.StartPhase3Apologizing()
	dkg.Finalize()

	m4 := makeApologyMsg()
	m4.Accuser = uint64(0)
	require.NotNil(t, dkg.HandleApologyMsg(m4))
}

func TestGetResultErrors(t *testing.T) {
	dkg := NewPureDKG(uint64(5), uint64(3), uint64(2), 1)
	_, err := dkg.ComputeResult()
	require.NotNil(t, err)
	_, _, err = dkg.StartPhase1Dealing()
	require.Nil(t, err)
	_, err = dkg.ComputeResult()
	require.NotNil(t, err)
	dkg.StartPhase2Accusing()
	_, err = dkg.ComputeResult()
	require.NotNil(t, err)
	dkg.StartPhase3Apologizing()
	_, err = dkg.ComputeResult()
	require.NotNil(t, err)
	dkg.Finalize()
	_, err = dkg.ComputeResult()
	require.NotNil(t, err)
}
