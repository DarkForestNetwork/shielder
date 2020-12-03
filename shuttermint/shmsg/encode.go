package shmsg

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
)

// URLEncodeMessage encodes Message as a string, which is safe to be used as part of an URL
func URLEncodeMessage(msg *MessageWithNonce) (string, error) {
	out, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// URLDecodeMessage decodes a Message from the given string
func URLDecodeMessage(encoded string) (*MessageWithNonce, error) {
	msg := MessageWithNonce{}
	out, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	if err := proto.Unmarshal(out, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Instead of relying on protocol buffers we simply send a signature, followed by the marshaled message

// Add a prefix to avoid accidentally signing data with special meaning in different context, in
// particular Ethereum transactions (c.f. EIP191 https://eips.ethereum.org/EIPS/eip-191).
var hashPrefix = []byte{0x19, 's', 'h', 'm', 's', 'g'}

// SignMessage signs the given Message with the given private key
func SignMessage(msg *MessageWithNonce, privkey *ecdsa.PrivateKey) ([]byte, error) {
	marshaled, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	hash := sha3.New256()
	_, err = hash.Write(hashPrefix)
	if err != nil {
		return nil, err
	}
	_, err = hash.Write(marshaled)
	if err != nil {
		return nil, err
	}

	h := hash.Sum(nil)
	signature, err := crypto.Sign(h, privkey)
	if err != nil {
		return nil, err
	}

	return append(signature, marshaled...), nil
}

// GetSigner returns the signer address of a signed message
func GetSigner(signedMessage []byte) (common.Address, error) {
	var signer common.Address
	if len(signedMessage) < crypto.SignatureLength {
		return signer, errors.New("message too short")
	}
	hash := sha3.New256()
	_, err := hash.Write(hashPrefix)
	if err != nil {
		return common.Address{}, err
	}

	_, err = hash.Write(signedMessage[crypto.SignatureLength:])
	if err != nil {
		return common.Address{}, err
	}
	h := hash.Sum(nil)
	pubkey, err := crypto.SigToPub(h, signedMessage[:crypto.SignatureLength])
	if err != nil {
		return signer, err
	}
	signer = crypto.PubkeyToAddress(*pubkey)
	return signer, nil
}

// GetMessage returns the unmarshalled Message of a signed message
func GetMessage(signedMessage []byte) (*MessageWithNonce, error) {
	msg := MessageWithNonce{}
	if err := proto.Unmarshal(signedMessage[crypto.SignatureLength:], &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
