package cmd

import (
	"fmt"
	"log"

	"shielder/shuttermint/keyper"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type KeyperConfig struct {
	ShielderURL string
	SigningKey     string
}

// keyperCmd represents the keyper command
var keyperCmd = &cobra.Command{
	Use:   "keyper",
	Short: "Run a shielder keyper",
	Run: func(cmd *cobra.Command, args []string) {
		keyperMain()
	},
}

func init() {
	rootCmd.AddCommand(keyperCmd)
	keyperCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
}

func readKeyperConfig() (KeyperConfig, error) {
	viper.SetEnvPrefix("KEYPER")
	viper.BindEnv("ShielderURL")
	viper.BindEnv("SigningKey")
	viper.SetDefault("ShielderURL", "http://localhost:26657")
	defer func() {
		if viper.ConfigFileUsed() != "" {
			log.Printf("Read config from %s", viper.ConfigFileUsed())
		}
	}()
	var err error
	kc := KeyperConfig{}

	viper.AddConfigPath("$HOME/.config/shielder")
	viper.SetConfigName("keyper")
	viper.SetConfigType("toml")
	viper.SetConfigFile(cfgFile)

	err = viper.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		// Config file not found
		if cfgFile != "" {
			return kc, err
		}
	} else if err != nil {
		return kc, err // Config file was found but another error was produced
	}

	err = viper.Unmarshal(&kc)
	return kc, err
}

func keyperMain() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

	kc, err := readKeyperConfig()
	if err != nil {
		panic(err)
	}

	privateKey, err := crypto.HexToECDSA(kc.SigningKey)
	if err != nil {
		panic(fmt.Errorf("bad signing key: %s '%s'", err, kc.SigningKey))
	}

	addr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	log.Printf("Starting keyper version %s with signing key %s, using %s", version, addr, kc.ShielderURL)
	k := keyper.NewKeyper(privateKey, kc.ShielderURL)
	err = k.Run()
	if err != nil {
		panic(err)
	}
}
