package cmd

import (
	"crypto/ed25519"
	"encoding/hex"
	"log"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shielder/shuttermint/cmd/shversion"
	"shielder/shuttermint/keyper"
)

// RawKeyperConfig contains raw, unvalidated configuration parameters.
type RawKeyperConfig struct {
	ShielderURL          string
	EthereumURL             string
	SigningKey              string
	ValidatorSeed           string
	EncryptionKey           string
	ConfigContract          string
	BatcherContract         string
	KeyBroadcastContract    string
	ExecutorContract        string
	DepositContract         string
	KeyperSlasher           string
	MainChainFollowDistance uint64
	ExecutionStaggering     uint64
	DKGPhaseLength          uint64
	DBDir                   string
}

// keyperCmd represents the keyper command.
var keyperCmd = &cobra.Command{
	Use:   "keyper",
	Short: "Run a Shielder keyper node",
	Long: `This command runs a keyper node. It will connect to both an Ethereum and a
Shielder node which have to be started separately in advance.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		keyperMain()
	},
}

func init() {
	keyperCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
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
	viper.BindEnv("MainChainFollowDistance")
	viper.BindEnv("ExecutionStaggering")
	viper.BindEnv("DKGPhaseLength")

	viper.SetDefault("ShielderURL", "http://localhost:26657")

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
		return emptyConfig, errors.Wrap(err, "bad signing key")
	}

	validatorSeed, err := hex.DecodeString(r.ValidatorSeed)
	if err != nil {
		return emptyConfig, errors.Wrap(err, "invalid validator seed")
	}
	if len(validatorSeed) != ed25519.SeedSize {
		return emptyConfig, errors.Errorf("invalid validator seed length %d (must be %d)", len(validatorSeed), ed25519.SeedSize)
	}
	validatorKey := ed25519.NewKeyFromSeed(validatorSeed)

	encryptionKeyECDSA, err := crypto.HexToECDSA(r.EncryptionKey)
	if err != nil {
		return emptyConfig, errors.Wrap(err, "bad encryption key")
	}
	encryptionKey := ecies.ImportECDSA(encryptionKeyECDSA)

	if !keyper.IsWebsocketURL(r.EthereumURL) {
		return emptyConfig, errors.Errorf("field EthereumURL must start with ws:// or wss://")
	}

	configContractAddress := common.HexToAddress(r.ConfigContract)
	if r.ConfigContract != configContractAddress.Hex() {
		return emptyConfig, errors.Errorf("field ConfigContract must be a valid checksummed address")
	}

	batcherContractAddress := common.HexToAddress(r.BatcherContract)
	if r.BatcherContract != batcherContractAddress.Hex() {
		return emptyConfig, errors.Errorf("field BatcherContract must be a valid checksummed address")
	}

	keyBroadcastContractAddress := common.HexToAddress(r.KeyBroadcastContract)
	if r.KeyBroadcastContract != keyBroadcastContractAddress.Hex() {
		return emptyConfig, errors.Errorf(
			"field KeyBroadcastContract must be a valid checksummed address",
		)
	}

	executorContractAddress := common.HexToAddress(r.ExecutorContract)
	if r.ExecutorContract != executorContractAddress.Hex() {
		return emptyConfig, errors.Errorf(
			"field ExecutorContract must be a valid checksummed address",
		)
	}

	depositContractAddress := common.HexToAddress(r.DepositContract)
	if r.DepositContract != depositContractAddress.Hex() {
		return emptyConfig, errors.Errorf(
			"field DepositContract must be a valid checksummed address",
		)
	}

	keyperSlasherAddress := common.HexToAddress(r.KeyperSlasher)
	if r.KeyperSlasher != keyperSlasherAddress.Hex() {
		return emptyConfig, errors.Errorf(
			"field KeyperSlasher must be a valid checksummed address",
		)
	}

	mainChainFollowDistance := r.MainChainFollowDistance
	executionStaggering := r.ExecutionStaggering
	dkgPhaseLength := r.DKGPhaseLength

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
		MainChainFollowDistance:     mainChainFollowDistance,
		ExecutionStaggering:         executionStaggering,
		DKGPhaseLength:              dkgPhaseLength,
		DBDir:                       r.DBDir,
	}, nil
}

func keyperMain() {
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
		shversion.Version(),
		kc.Address().Hex(),
		kc.ShielderURL,
		kc.EthereumURL,
	)
	kpr := keyper.NewKeyper(kc)
	err = kpr.LoadState()
	if err != nil {
		log.Fatalf("Error in LoadState: %+v", err)
	}
	log.Printf("Loaded state with %d actions, %s", len(kpr.State.Actions), kpr.ShortInfo())
	err = kpr.Run()
	if err != nil {
		log.Fatalf("Error in Run: %+v", err)
	}
}
