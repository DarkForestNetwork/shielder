package main

import (
	"encoding/base64"
	"fmt"

	"shielder/shuttermint/shmsg"
	"github.com/ethereum/go-ethereum/crypto"
)

func makeMessage() *shmsg.Message {
	return &shmsg.Message{
		Payload: &shmsg.Message_PublicKeyCommitment{
			PublicKeyCommitment: &shmsg.PublicKeyCommitment{
				BatchId:    "foo-1",
				Commitment: []byte("foobar"),
				Signature:  []byte("signature"),
			},
		},
	}
}

func main() {
	privateKey, err := crypto.HexToECDSA("fad9c8855b740a0b7ed4c221dbad0f33a83a49cad6b3fe8d5817ac83d38b6a19")
	if err != nil {
		panic(err)
	}
	msg := makeMessage()
	signedMessage, err := shmsg.SignMessage(msg, privateKey)
	if err != nil {
		panic(err)
	}
	fmt.Println("Msg:", base64.RawURLEncoding.EncodeToString(signedMessage))
}
