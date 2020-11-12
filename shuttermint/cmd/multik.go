package cmd

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/spf13/cobra"

	"shielder/shuttermint/keyper"
	"shielder/shuttermint/sandbox"
)

var multikFlags struct {
	ShielderURL string
	EthereumURL    string
}

// multikCmd represents the multik command
var multikCmd = &cobra.Command{
	Use:   "multik",
	Short: "Run multiple keypers in a single process for testing purposes",
	Run: func(cmd *cobra.Command, args []string) {
		multikMain()
	},
}

func init() {
	rootCmd.AddCommand(multikCmd)
	multikCmd.PersistentFlags().StringVarP(
		&multikFlags.ShielderURL,
		"shuttermint-url",
		"s",
		"http://localhost:26657",
		"Shielder RPC URL",
	)
	multikCmd.PersistentFlags().StringVarP(
		&multikFlags.EthereumURL,
		"ethereum-url",
		"e",
		"ws://localhost:8545/websocket",
		"Ethereum RPC URL",
	)
}

func multikMain() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	log.Printf("Starting multik version %s", version)
	baseConfig := keyper.KeyperConfig{
		ShielderURL:              multikFlags.ShielderURL,
		EthereumURL:                 multikFlags.EthereumURL,
		ConfigContractAddress:       common.HexToAddress("0x07a457d878BF363E0Bb5aa0B096092f941e19962"),
		KeyBroadcastContractAddress: common.HexToAddress("0xFA33c8EF8b5c4f3003361c876a298D1DB61ccA4e"),
		BatcherContractAddress:      common.HexToAddress("0x27D44c7337ce4D67b7cd573e9c36bDEED2b2162a"),
		ExecutorContractAddress:     common.HexToAddress("0x5d18dED3c0A476fCbc9E67Fc1C613cfc5DD0d34B"),
	}

	var signingKeys [3]*ecdsa.PrivateKey
	var validatorKeys [3]ed25519.PrivateKey
	var encryptionKeys [3]*ecies.PrivateKey
	var keypers [3]common.Address
	for i := 0; i < 3; i++ {
		k := sandbox.GanacheKey(i)
		signingKeys[i] = k
		keypers[i] = crypto.PubkeyToAddress(k.PublicKey)

		validatorSeed := make([]byte, 32)
		copy(validatorSeed, keypers[i].Bytes())
		validatorKeys[i] = ed25519.NewKeyFromSeed(validatorSeed)

		// reusing the signing key is fine for tests
		encryptionKeys[i] = ecies.ImportECDSA(k)
	}

	for i := 0; i < 3; i++ {
		go func(signingKey *ecdsa.PrivateKey, validatorKey ed25519.PrivateKey, encryptionKey *ecies.PrivateKey) {
			config := baseConfig
			config.SigningKey = signingKey
			config.ValidatorKey = validatorKey
			config.EncryptionKey = encryptionKey

			kpr := keyper.NewKeyper(config)
			err := kpr.Run()
			if err != nil {
				panic(err)
			}
		}(signingKeys[i], validatorKeys[i], encryptionKeys[i])
	}
	time.Sleep(time.Hour)
}
