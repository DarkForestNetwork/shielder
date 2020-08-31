package main

import (
	"shielder/shuttermint/keyper"
	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	privateKey, err := crypto.HexToECDSA("fad9c8855b740a0b7ed4c221dbad0f33a83a49cad6b3fe8d5817ac83d38b6a19")
	if err != nil {
		panic(err)
	}
	err = keyper.Keyper{
		SigningKey:     privateKey,
		ShielderURL: "http://localhost:26657",
	}.Run()
	if err != nil {
		panic(err)
	}
}
