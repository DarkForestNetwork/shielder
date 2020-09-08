package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version string = "(unknown)"
var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "shuttermint",
	Short: "shuttermint runs the shielder tendermint app or a shielder keyper",
	Long:  `This should become a long description of what shielder is`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
