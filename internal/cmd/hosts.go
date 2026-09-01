package cmd

import (
	"fmt"

	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "List the hosts in the manifest and how many repos each carries",
	RunE:  runHosts,
}

func init() {
	rootCmd.AddCommand(hostsCmd)
}

func runHosts(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}

	this := host.Name()
	fmt.Printf("  %-14s %8s %10s\n", dim("HOST"), dim("ACTIVE"), dim("ARCHIVED"))
	for _, h := range m.Hosts() {
		var active, archived int
		for _, r := range m.ReposForHost(h) {
			if r.IsArchived() {
				archived++
			} else {
				active++
			}
		}
		// Pad before styling — ANSI escapes would break %-14s alignment.
		label := fmt.Sprintf("%-14s", h)
		if h == this {
			label = green(fmt.Sprintf("%-14s", h+" *"))
		}
		fmt.Printf("  %s %8d %10s\n", label, active, dim(fmt.Sprint(archived)))
	}
	if m.Find(this) == -1 && len(m.ReposForHost(this)) == 0 {
		fmt.Printf("\n  %s %s has no repos yet — run 'repoman init'\n", yellow("!"), this)
	}
	return nil
}
