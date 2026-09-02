package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "repoman",
	Short: "Multi-machine git repository manager",
	Long:  "Manage ~200 git repos across multiple machines with per-host manifests and restic backup.",
	// A runtime failure is not a usage error; dumping the flag list on top of
	// "the drive is unmounted" buries the message that matters, especially in
	// a log written by a scheduled job.
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}
