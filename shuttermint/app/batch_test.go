package app

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

var addresses [10]common.Address

func init() {
	for i := 0; i < 10; i++ {
		addresses[i] = common.BigToAddress(big.NewInt(int64(i)))
	}
}

// TestAddpublickeycommitment tests the basic functionality of AddPublicKeyCommitment
func TestAddPublicKeyCommitment(t *testing.T) {
	batchKeys := BatchKeys{Config: &BatchConfig{Keypers: addresses[:5]}}
	t.Logf("batch: %+v", batchKeys)
	err := batchKeys.AddPublicKeyCommitment(PublicKeyCommitment{Sender: addresses[0]})
	if err != nil {
		t.Fatalf("could not add public key commitment: %s", err)
	}

	if len(batchKeys.Commitments) != 1 {
		t.Fatalf("wrong number of commitments: %s", batchKeys.Commitments)
	}

	err = batchKeys.AddPublicKeyCommitment(PublicKeyCommitment{Sender: addresses[0]})
	if err == nil {
		t.Fatalf("no error")
	}
	t.Logf("received expected error: %s", err)
	if len(batchKeys.Commitments) != 1 {
		t.Fatalf("wrong number of commitments: %s", batchKeys.Commitments)
	}

	err = batchKeys.AddPublicKeyCommitment(PublicKeyCommitment{Sender: addresses[6]})
	if err == nil {
		t.Fatalf("no error")
	}

	t.Logf("received expected error: %s", err)
	if len(batchKeys.Commitments) != 1 {
		t.Fatalf("wrong number of commitments: %s", batchKeys.Commitments)
	}
}

func TestAddSecretShare(t *testing.T) {
	key1, err := crypto.GenerateKey()
	require.Nil(t, err, "could not generate key")

	key2, err := crypto.GenerateKey()
	require.Nil(t, err, "could not generate key")

	batchKeys := BatchKeys{Config: &BatchConfig{Keypers: addresses[:5]}}
	t.Logf("batch: %+v", batchKeys)
	err = batchKeys.AddPublicKeyCommitment(PublicKeyCommitment{
		Sender: addresses[0],
		Pubkey: crypto.FromECDSAPub(&key1.PublicKey)})
	require.Nil(t, err)

	// this should fail because we didn't provide a public key
	err = batchKeys.AddSecretShare(SecretShare{Sender: addresses[1], Privkey: crypto.FromECDSA(key1)})
	require.NotNil(t, err, "added secret share without providing public key first")
	t.Logf("received expected error: %s", err)

	// this should fail because we use a non-matching private key
	err = batchKeys.AddSecretShare(SecretShare{Sender: addresses[0], Privkey: crypto.FromECDSA(key2)})
	require.NotNil(t, err, "added secret share with non-matching key")
	t.Logf("received expected error: %s", err)

	// this should succeed
	err = batchKeys.AddSecretShare(SecretShare{Sender: addresses[0], Privkey: crypto.FromECDSA(key1)})
	require.Nil(t, err, "could not add secret share: %s", err)

	err = batchKeys.AddSecretShare(SecretShare{Sender: addresses[0], Privkey: crypto.FromECDSA(key1)})
	require.NotNil(t, err, "providing a secret key a second time should fail")
}
