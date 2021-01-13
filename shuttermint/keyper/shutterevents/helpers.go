package shielderevents

import (
	"github.com/ethereum/go-ethereum/common"
	abcitypes "github.com/tendermint/tendermint/abci/types"

	shcrypto "shielder/shuttermint/crypto"
)

//
// Encoding/decoding helpers
//

func newAddressPair(key string, value common.Address) abcitypes.EventAttribute {
	return abcitypes.EventAttribute{
		Key:   []byte(key),
		Value: encodeAddress(value),
		Index: true,
	}
}

func newAddressesPair(key string, value []common.Address) abcitypes.EventAttribute {
	return abcitypes.EventAttribute{
		Key:   []byte(key),
		Value: encodeAddresses(value),
	}
}

func newByteSequencePair(key string, value [][]byte) abcitypes.EventAttribute {
	return abcitypes.EventAttribute{
		Key:   []byte(key),
		Value: encodeByteSequence(value),
	}
}

func newUintPair(key string, value uint64) abcitypes.EventAttribute {
	return abcitypes.EventAttribute{
		Key:   []byte(key),
		Value: encodeUint64(value),
		Index: true,
	}
}

func newGammas(key string, gammas *shcrypto.Gammas) abcitypes.EventAttribute {
	return abcitypes.EventAttribute{
		Key:   []byte(key),
		Value: encodeGammas(gammas),
	}
}
