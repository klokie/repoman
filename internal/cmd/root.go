package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "repoman",
	Short: "Multi-machine git repository manager",
	Long:  "Manage ~200 git repos across multiple machines with per-host manifests and restic backup.",
}

func Execute() error {
	return rootCmd.Execute()
}
