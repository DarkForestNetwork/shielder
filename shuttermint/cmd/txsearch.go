package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"

	"shielder/shuttermint/keyper/observe"
	"shielder/shuttermint/keyper/shielderevents"
)

var txsearchFlags struct {
	ShielderURL       string
	FromHeight, ToHeight int64
}

var txsearchCmd = &cobra.Command{
	Use:   "txsearch",
	Short: "Search for shuttermint transactions",
	Long: `This command queries transactions from a running shuttermint node and prints them to
	stdout.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		txsearchMain()
	},
}

func init() {
	txsearchCmd.PersistentFlags().StringVarP(
		&txsearchFlags.ShielderURL,
		"shuttermint-url",
		"s",
		"http://localhost:26657",
		"Shielder RPC URL",
	)
	txsearchCmd.PersistentFlags().Int64VarP(
		&txsearchFlags.FromHeight,
		"from-height",
		"",
		0,
		"search txs from this height",
	)
	txsearchCmd.PersistentFlags().Int64VarP(
		&txsearchFlags.ToHeight,
		"to-height",
		"",
		-1,
		"search txs till this height",
	)
}

func txsearchShielder(shuttermintURL string, fromHeight, toHeight int64) {
	var cl client.Client
	cl, err := http.New(shuttermintURL, "/websocket")
	if err != nil {
		panic(err)
	}

	s := observe.NewShielder()
	if toHeight == -1 {
		toHeight, err = s.GetLastCommittedHeight(context.Background(), cl)
		if err != nil {
			panic(err)
		}
	}

	query := fmt.Sprintf("tx.height >= %d and tx.height <= %d", fromHeight, toHeight)

	perPage := 100
	page := 1
	ctx := context.Background()
	for {
		res, err := cl.TxSearch(ctx, query, false, &page, &perPage, "")
		if err != nil {
			panic(err)
		}
		for _, tx := range res.Txs {
			fmt.Printf("==> TX height=%d hash=%s\n", tx.Height, tx.Hash)
			events := tx.TxResult.GetEvents()
			for _, ev := range events {
				x, err := shielderevents.MakeEvent(ev, tx.Height)
				if err != nil {
					log.Printf("Error: malformed event: %+v ev=%+v", err, ev)
				} else {
					fmt.Printf("    %#v\n", x)
				}
			}
		}
		if page*perPage >= res.TotalCount {
			break
		}
		page++
	}
}

func txsearchMain() {
	txsearchShielder(txsearchFlags.ShielderURL, txsearchFlags.FromHeight, txsearchFlags.ToHeight)
}
