package cmd

import (
	"log"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shielder/shuttermint/cmd/shversion"
	"shielder/shuttermint/keyper"
)

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

func readKeyperConfig() (keyper.Config, error) {
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
	config := keyper.Config{}

	viper.AddConfigPath("$HOME/.config/shielder")
	viper.SetConfigName("keyper")
	viper.SetConfigType("toml")
	viper.SetConfigFile(cfgFile)

	err = viper.ReadInConfig()

	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		// Config file not found
		if cfgFile != "" {
			return config, err
		}
	} else if err != nil {
		return config, err // Config file was found but another error was produced
	}
	err = config.Unmarshal(viper.GetViper())

	if err != nil {
		return config, err
	}

	if !filepath.IsAbs(config.DBDir) {
		r := filepath.Dir(viper.ConfigFileUsed())
		dbdir, err := filepath.Abs(filepath.Join(r, config.DBDir))
		if err != nil {
			return config, err
		}
		config.DBDir = dbdir
	}

	if !keyper.IsWebsocketURL(config.EthereumURL) {
		return config, errors.Errorf("field EthereumURL must start with ws:// or wss://")
	}

	return config, err
}

func keyperMain() {
	kc, err := readKeyperConfig()
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
