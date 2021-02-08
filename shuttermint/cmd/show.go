package cmd

import (
	"context"

	"github.com/kr/pretty"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"

	"shielder/shuttermint/keyper/observe"
)

var showFlags struct {
	ShielderURL string
	Height         int64
}

// keyperCmd represents the keyper command
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show shielder internal state",
	Run: func(cmd *cobra.Command, args []string) {
		showMain()
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
	showCmd.PersistentFlags().StringVarP(
		&showFlags.ShielderURL,
		"shuttermint-url",
		"s",
		"http://localhost:26657",
		"Shielder RPC URL",
	)
	showCmd.PersistentFlags().Int64VarP(
		&showFlags.Height,
		"height",
		"",
		-1,
		"target height",
	)
}

func showShielder(shuttermintURL string, height int64) {
	var cl client.Client
	cl, err := http.New(shuttermintURL, "/websocket")
	if err != nil {
		panic(err)
	}

	s := observe.NewShielder()
	latestBlock, err := cl.Block(context.Background(), nil)
	if err != nil {
		panic(err)
	}
	if latestBlock.Block == nil {
		panic("empty shielder blockchain")
	}
	if height == -1 {
		height = latestBlock.Block.Height
	}
	s, err = s.SyncToHeight(context.Background(), cl, height)
	if err != nil {
		panic(err)
	}
	pretty.Println("Synced:", s)
}

func showMain() {
	showShielder(showFlags.ShielderURL, showFlags.Height)
}
