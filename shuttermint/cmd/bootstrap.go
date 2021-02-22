package cmd

import (
	"context"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"

	"shielder/shuttermint/contract"
	"shielder/shuttermint/keyper"
	"shielder/shuttermint/sandbox"
	"shielder/shuttermint/shmsg"
)

var bootstrapFlags struct {
	ShielderURL   string
	EthereumURL      string
	BatchConfigIndex int
	ConfigContract   string
	ContractsPath    string
	SigningKey       string
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap Shielder by submitting the initial batch config",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		bootstrap()
	},
}

func getConfigContractAddress() common.Address {
	var configContractAddress common.Address
	if bootstrapFlags.ContractsPath != "" {
		contracts, err := sandbox.LoadContractsJSON(bootstrapFlags.ContractsPath)
		if err != nil {
			log.Fatalf("could not read contracts: %s", err)
		}
		configContractAddress = common.HexToAddress(contracts.ConfigContract)
	} else if bootstrapFlags.ConfigContract != "" {
		configContractAddress = common.HexToAddress(bootstrapFlags.ConfigContract)
		if bootstrapFlags.ConfigContract != configContractAddress.Hex() {
			log.Fatalf("Invalid config contract address %s", bootstrapFlags.ConfigContract)
		}
	} else {
		log.Fatalf("must specify either --contracts or -c flag")
	}
	return configContractAddress
}

func init() {
	bootstrapCmd.PersistentFlags().StringVarP(
		&bootstrapFlags.ShielderURL,
		"shuttermint-url",
		"s",
		"http://localhost:26657",
		"Shielder RPC URL",
	)
	bootstrapCmd.PersistentFlags().StringVarP(
		&bootstrapFlags.EthereumURL,
		"ethereum-url",
		"e",
		"ws://localhost:8545/websocket",
		"Ethereum RPC URL",
	)
	bootstrapCmd.PersistentFlags().IntVarP(
		&bootstrapFlags.BatchConfigIndex,
		"index",
		"i",
		-1,
		"index of the batch config to bootstrap with (use latest if negative)",
	)

	bootstrapCmd.PersistentFlags().StringVarP(
		&bootstrapFlags.ConfigContract,
		"config-contract",
		"c",
		"",
		"address of the contract from which to fetch config",
	)
	bootstrapCmd.Flags().StringVarP(
		&bootstrapFlags.ContractsPath,
		"contracts",
		"", // can't reuse "c" here
		"",
		"read config contract address from the given contracts.json file",
	)

	bootstrapCmd.PersistentFlags().StringVarP(
		&bootstrapFlags.SigningKey,
		"signing-key",
		"k",
		"",
		"private key of the keyper to send the message with",
	)
	bootstrapCmd.MarkPersistentFlagRequired("signing-key")
}

func bootstrap() {
	ethcl, err := ethclient.Dial(bootstrapFlags.EthereumURL)
	if err != nil {
		log.Fatalf("Error connecting to Ethereum node: %v", err)
	}

	shmcl, err := http.New(bootstrapFlags.ShielderURL, "/websocket")
	if err != nil {
		log.Fatalf("Error connecting to Shielder node: %v", err)
	}

	signingKey, err := crypto.HexToECDSA(bootstrapFlags.SigningKey)
	if err != nil {
		log.Fatalf("Invalid signing key: %v", err)
	}
	configContractAddress := getConfigContractAddress()
	configContract, err := contract.NewConfigContract(configContractAddress, ethcl)
	if err != nil {
		log.Fatalf("Failed to instantiate ConfigContract: %v", err)
	}

	header, err := ethcl.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to fetch block header: %v", err)
	}
	opts := &bind.CallOpts{
		Pending:     false,
		BlockNumber: header.Number,
		Context:     context.Background(),
	}

	var batchConfigIndex uint64
	if bootstrapFlags.BatchConfigIndex < 0 {
		numConfigs, err := configContract.NumConfigs(opts)
		if err != nil {
			log.Fatalf("Failed to fetch number of configs: %v", err)
		}
		if numConfigs == 0 {
			log.Fatal("no configs found")
		}
		batchConfigIndex = numConfigs - 1
	} else {
		batchConfigIndex = uint64(bootstrapFlags.BatchConfigIndex)
	}
	bc, err := configContract.GetConfigByIndex(opts, batchConfigIndex)
	if err != nil {
		log.Fatalf("Failed to fetch config at index %d: %v", batchConfigIndex, err)
	}
	keypers := bc.Keypers

	ms := keyper.NewRPCMessageSender(shmcl, signingKey)
	batchConfigMsg := shmsg.NewBatchConfig(
		bc.StartBatchIndex,
		keypers,
		bc.Threshold,
		configContractAddress,
		batchConfigIndex,
		false,
		false,
	)

	err = ms.SendMessage(context.Background(), batchConfigMsg)
	if err != nil {
		log.Fatalf("Failed to send batch config message: %v", err)
	}

	batchConfigStartedMsg := shmsg.NewBatchConfigStarted(batchConfigIndex)
	err = ms.SendMessage(context.Background(), batchConfigStartedMsg)
	if err != nil {
		log.Fatalf("Failed to send start message: %v", err)
	}

	log.Println("Submitted bootstrapping transaction")
	log.Printf("Config index: %d", batchConfigIndex)
	log.Printf("StartBatchIndex: %d", bc.StartBatchIndex)
	log.Printf("Threshold: %d", bc.Threshold)
	log.Printf("Num Keypers: %d", len(keypers))
}
