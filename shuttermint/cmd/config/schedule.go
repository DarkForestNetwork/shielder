package config

import (
	"context"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"shielder/shuttermint/medley"
	"shielder/shuttermint/sandbox"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Schedule the next batch config",
	Long: `This command schedules the next batch config. The next batch config can be
configured using the 'shuttermint config set-next' command and queried with
'shuttermint config query -i next'.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		sandbox.ExitIfError(processConfigFlags(ctx))
		if flag, err := validateScheduleFlags(); err != nil {
			sandbox.ExitIfError(errors.Wrapf(err, "invalid value for flag %s", flag))
		}
		sandbox.ExitIfError(schedule(ctx))
	},
}

var scheduleFlags struct {
	Key string
}

func init() {
	scheduleCmd.PersistentFlags().StringVarP(
		&scheduleFlags.Key,
		keyFlagName,
		"k",
		"",
		"private key of the owner",
	)
	sandbox.MarkFlagRequired(scheduleCmd, keyFlagName)
}

func validateScheduleFlags() (string, error) {
	if err := sandbox.ValidatePrivateKey(scheduleFlags.Key); err != nil {
		return keyFlagName, err
	}

	return "", nil
}

func schedule(ctx context.Context) error {
	key, err := crypto.HexToECDSA(scheduleFlags.Key)
	if err != nil {
		return err // should be already checked during parameter validation
	}
	o, err := sandbox.InitTransactOpts(ctx, client, key)
	if err != nil {
		return err
	}

	tx, err := configContract.ScheduleNextConfig(o)
	if err != nil {
		return err
	}
	_, err = medley.WaitMined(ctx, client, tx.Hash())
	if err != nil {
		return err
	}

	return nil
}
