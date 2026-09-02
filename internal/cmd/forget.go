package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var forgetCmd = &cobra.Command{
	Use:   "forget",
	Short: "Apply the snapshot retention policy (dry run unless --apply)",
	Long: `Thin out old snapshots per host, keeping a daily/weekly/monthly ladder.

Dry run by default, and it never prunes unless asked: forgetting a snapshot only
drops the reference, while pruning deletes the data for good. Two steps, so a
mistaken policy can still be undone before the data goes.`,
	RunE: runForget,
}

var (
	forgetApply   bool
	forgetPrune   bool
	forgetDaily   int
	forgetWeekly  int
	forgetMonthly int
	forgetLast    int
)

func init() {
	forgetCmd.Flags().BoolVar(&forgetApply, "apply", false, "actually forget (default: dry run)")
	forgetCmd.Flags().BoolVar(&forgetPrune, "prune", false, "also delete the unreferenced data (irreversible)")
	forgetCmd.Flags().IntVar(&forgetDaily, "keep-daily", 0, "override defaults.keep_daily")
	forgetCmd.Flags().IntVar(&forgetWeekly, "keep-weekly", 0, "override defaults.keep_weekly")
	forgetCmd.Flags().IntVar(&forgetMonthly, "keep-monthly", 0, "override defaults.keep_monthly")
	forgetCmd.Flags().IntVar(&forgetLast, "keep-last", 0, "override defaults.keep_last")
	rootCmd.AddCommand(forgetCmd)
}

func runForget(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	repo, passFile, err := resticTarget(m)
	if err != nil {
		return err
	}

	pick := func(flag, configured, fallback int) int {
		if flag > 0 {
			return flag
		}
		if configured > 0 {
			return configured
		}
		return fallback
	}
	daily := pick(forgetDaily, m.Defaults.KeepDaily, 7)
	weekly := pick(forgetWeekly, m.Defaults.KeepWeekly, 4)
	monthly := pick(forgetMonthly, m.Defaults.KeepMonthly, 6)
	last := pick(forgetLast, m.Defaults.KeepLast, 3)

	// Group by host so one machine backing up often cannot age out another's
	// only snapshot.
	resticArgs := []string{"-r", repo, "forget", "--group-by", "host",
		"--keep-last", strconv.Itoa(last),
		"--keep-daily", strconv.Itoa(daily),
		"--keep-weekly", strconv.Itoa(weekly),
		"--keep-monthly", strconv.Itoa(monthly),
	}
	if !forgetApply {
		resticArgs = append(resticArgs, "--dry-run")
	}
	if forgetPrune && forgetApply {
		resticArgs = append(resticArgs, "--prune")
	}

	fmt.Printf("Policy: keep last %d, daily %d, weekly %d, monthly %d — per host\n", last, daily, weekly, monthly)
	if !forgetApply {
		fmt.Printf("%s\n\n", dim("dry run — pass --apply to act, and --prune to reclaim the space"))
	} else if !forgetPrune {
		fmt.Printf("%s\n\n", dim("forgetting only; the data stays until you run with --prune"))
	}

	c := exec.Command("restic", resticArgs...)
	c.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+passFile)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
