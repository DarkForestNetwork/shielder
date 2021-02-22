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
	Short: "Show the internal state of a Shielder node",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		showMain()
	},
}

func init() {
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
	if height == -1 {
		height, err = s.LastCommittedHeight(context.Background(), cl)
		if err != nil {
			panic(err)
		}
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
