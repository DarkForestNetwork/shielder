package cmd

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shielder/shuttermint/keyper"
)

// RawKeyperConfig contains raw, unvalidated configuration parameters
type RawKeyperConfig struct {
	ShielderURL       string
	EthereumURL          string
	SigningKey           string
	ValidatorSeed        string
	EncryptionKey        string
	ConfigContract       string
	BatcherContract      string
	KeyBroadcastContract string
	ExecutorContract     string
	DepositContract      string
	KeyperSlasher        string
	ExecutionStaggering  string
	DBDir                string
}

// keyperCmd represents the keyper command
var keyperCmd = &cobra.Command{
	Use:   "keyper",
	Short: "Run a shielder keyper",
	Run: func(cmd *cobra.Command, args []string) {
		keyperMain()
	},
}

// keyperCmd represents the keyper2 command
var keyper2Cmd = &cobra.Command{
	Use:   "keyper2",
	Short: "Run a shielder keyper2",
	Run: func(cmd *cobra.Command, args []string) {
		keyper2Main()
	},
}
var interactive = false

func init() {
	rootCmd.AddCommand(keyperCmd)
	keyperCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")

	rootCmd.AddCommand(keyper2Cmd)
	keyper2Cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
	keyper2Cmd.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false, "interactive mode for debugging")
}

func readKeyperConfig() (RawKeyperConfig, error) {
	viper.SetEnvPrefix("KEYPER")
	viper.BindEnv("ShielderURL")
	viper.BindEnv("EthereumURL")
	viper.BindEnv("SigningKey")
	viper.BindEnv("ValidatorSeed")
	viper.BindEnv("EncryptionKey")
	viper.BindEnv("ConfigContract")
	viper.BindEnv("BatcherContract")
	viper.BindEnv("KeyBroadcastContract")
	viper.BindEnv("ExecutorContract")
	viper.BindEnv("DepositContract")
	viper.BindEnv("KeyperSlasher")
	viper.BindEnv("ExecutionStaggering")

	viper.SetDefault("ShielderURL", "http://localhost:26657")
	viper.SetDefault("EthereumURL", "ws://localhost:8545/websocket")

	defer func() {
		if viper.ConfigFileUsed() != "" {
			log.Printf("Read config from %s", viper.ConfigFileUsed())
		}
	}()
	var err error
	rkc := RawKeyperConfig{}

	viper.AddConfigPath("$HOME/.config/shielder")
	viper.SetConfigName("keyper")
	viper.SetConfigType("toml")
	viper.SetConfigFile(cfgFile)

	err = viper.ReadInConfig()

	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		// Config file not found
		if cfgFile != "" {
			return rkc, err
		}
	} else if err != nil {
		return rkc, err // Config file was found but another error was produced
	}
	err = viper.Unmarshal(&rkc)
	if err != nil {
		return rkc, err
	}

	if !filepath.IsAbs(rkc.DBDir) {
		r := filepath.Dir(viper.ConfigFileUsed())
		dbdir, err := filepath.Abs(filepath.Join(r, rkc.DBDir))
		if err != nil {
			return rkc, err
		}
		rkc.DBDir = dbdir
	}

	return rkc, err
}

func ValidateKeyperConfig(r RawKeyperConfig) (keyper.KeyperConfig, error) {
	emptyConfig := keyper.KeyperConfig{}

	signingKey, err := crypto.HexToECDSA(r.SigningKey)
	if err != nil {
		return emptyConfig, fmt.Errorf("bad signing key: %w", err)
	}

	validatorSeed, err := hex.DecodeString(r.ValidatorSeed)
	if err != nil {
		return emptyConfig, fmt.Errorf("invalid validator seed: %w", err)
	}
	if len(validatorSeed) != ed25519.SeedSize {
		return emptyConfig, fmt.Errorf("invalid validator seed length %d (must be %d)", len(validatorSeed), ed25519.SeedSize)
	}
	validatorKey := ed25519.NewKeyFromSeed(validatorSeed)

	encryptionKeyECDSA, err := crypto.HexToECDSA(r.EncryptionKey)
	if err != nil {
		return emptyConfig, fmt.Errorf("bad encryption key: %w", err)
	}
	encryptionKey := ecies.ImportECDSA(encryptionKeyECDSA)

	if !keyper.IsWebsocketURL(r.EthereumURL) {
		return emptyConfig, fmt.Errorf("field EthereumURL must start with ws:// or wss://")
	}

	configContractAddress := common.HexToAddress(r.ConfigContract)
	if r.ConfigContract != configContractAddress.Hex() {
		return emptyConfig, fmt.Errorf("field ConfigContract must be a valid checksummed address")
	}

	batcherContractAddress := common.HexToAddress(r.BatcherContract)
	if r.BatcherContract != batcherContractAddress.Hex() {
		return emptyConfig, fmt.Errorf("field BatcherContract must be a valid checksummed address")
	}

	keyBroadcastContractAddress := common.HexToAddress(r.KeyBroadcastContract)
	if r.KeyBroadcastContract != keyBroadcastContractAddress.Hex() {
		return emptyConfig, fmt.Errorf(
			"KeyBroadcastContract must be a valid checksummed address",
		)
	}

	executorContractAddress := common.HexToAddress(r.ExecutorContract)
	if r.ExecutorContract != executorContractAddress.Hex() {
		return emptyConfig, fmt.Errorf(
			"ExecutorContract must be a valid checksummed address",
		)
	}

	depositContractAddress := common.HexToAddress(r.DepositContract)
	if r.DepositContract != depositContractAddress.Hex() {
		return emptyConfig, fmt.Errorf(
			"DepositContract must be a valid checksummed address",
		)
	}

	keyperSlasherAddress := common.HexToAddress(r.KeyperSlasher)
	if r.KeyperSlasher != keyperSlasherAddress.Hex() {
		return emptyConfig, fmt.Errorf(
			"KeyperSlasher must be a valid checksummed address",
		)
	}

	executionStaggering, err := strconv.ParseUint(r.ExecutionStaggering, 10, 64)
	if err != nil {
		return emptyConfig, fmt.Errorf("error parsing ExecutionStaggering as non-negative integer: %w", err)
	}

	return keyper.KeyperConfig{
		ShielderURL:              r.ShielderURL,
		EthereumURL:                 r.EthereumURL,
		SigningKey:                  signingKey,
		ValidatorKey:                validatorKey,
		EncryptionKey:               encryptionKey,
		ConfigContractAddress:       configContractAddress,
		BatcherContractAddress:      batcherContractAddress,
		KeyBroadcastContractAddress: keyBroadcastContractAddress,
		ExecutorContractAddress:     executorContractAddress,
		DepositContractAddress:      depositContractAddress,
		KeyperSlasherAddress:        keyperSlasherAddress,
		ExecutionStaggering:         executionStaggering,
		DBDir:                       r.DBDir,
	}, nil
}

func keyperMain() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	rkc, err := readKeyperConfig()
	if err != nil {
		log.Fatalf("Error reading the configuration file: %s\nPlease check your configuration.", err)
	}

	kc, err := ValidateKeyperConfig(rkc)
	if err != nil {
		log.Fatalf("Error: %s\nPlease check your configuration", err)
	}

	log.Printf(
		"Starting keyper version %s with signing key %s, using %s for Shielder and %s for Ethereum",
		version,
		kc.Address().Hex(),
		kc.ShielderURL,
		kc.EthereumURL,
	)
	kpr := keyper.NewKeyper(kc)
	err = kpr.Run()
	if err != nil {
		panic(err)
	}
}

func keyper2Main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	rkc, err := readKeyperConfig()
	if err != nil {
		log.Fatalf("Error reading the configuration file: %s\nPlease check your configuration.", err)
	}

	kc, err := ValidateKeyperConfig(rkc)
	if err != nil {
		log.Fatalf("Error: %s\nPlease check your configuration", err)
	}

	log.Printf(
		"Starting keyper version %s with signing key %s, using %s for Shielder and %s for Ethereum",
		version,
		kc.Address().Hex(),
		kc.ShielderURL,
		kc.EthereumURL,
	)
	kpr := keyper.NewKeyper2(kc)
	err = kpr.LoadState()
	if err != nil {
		panic(err)
	}
	log.Printf("loaded state: %s", kpr.ShortInfo())
	kpr.Interactive = interactive
	err = kpr.Run()
	if err != nil {
		panic(err)
	}
}
