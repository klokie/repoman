package cmd

import (
	"fmt"

	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Drop manifest entries that no longer belong to any host",
	Long: `Remove repos that every host has unassigned.

'unassign' deliberately leaves the entry behind — a repo removed from one
machine usually still lives on another. Once the last host lets go, the entry
is just noise, and this is what clears it.`,
	RunE: runPrune,
}

var pruneDryRun bool

func init() {
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "list what would be dropped without writing the manifest")
	rootCmd.AddCommand(pruneCmd)
}

func runPrune(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}

	kept := make([]manifest.Repo, 0, len(m.Repos))
	var dropped []manifest.Repo
	for _, r := range m.Repos {
		if len(r.Hosts) == 0 {
			dropped = append(dropped, r)
			continue
		}
		kept = append(kept, r)
	}

	if len(dropped) == 0 {
		fmt.Println("Nothing to prune — every entry belongs to at least one host")
		return nil
	}
	for _, r := range dropped {
		fmt.Printf("  %s %-32s %s\n", yellow("-"), r.Name, dim(r.Remote))
	}
	if pruneDryRun {
		fmt.Printf("\ndry run: %d entries would be dropped\n", len(dropped))
		return nil
	}

	m.Repos = kept
	if err := manifest.Save(m); err != nil {
		return err
	}
	fmt.Printf("\nDropped %d entries, %d remain — run 'repoman sync-manifest' to share it\n", len(dropped), len(m.Repos))
	return nil
}
