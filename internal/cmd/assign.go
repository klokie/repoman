package cmd

import (
	"fmt"
	"strings"

	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var assignCmd = &cobra.Command{
	Use:   "assign <repo|tag>...",
	Short: "Claim repos for this host so 'repoman clone' will bring them down",
	Long: `Add this host to one or more manifest entries, by repo name or by tag.

This is how a repo that lives on one machine gets onto another: assign it, sync
the manifest, then clone.

  repoman assign acme-api hermes
  repoman assign --tag personal
  repoman assign --host metalmark hermes`,
	Args: cobra.ArbitraryArgs,
	RunE: runAssign,
}

var unassignCmd = &cobra.Command{
	Use:   "unassign <repo>...",
	Short: "Remove this host from repos (the local clone is left alone)",
	Args:  cobra.ArbitraryArgs,
	RunE:  runUnassign,
}

var (
	assignHost string
	assignTag  string
)

func init() {
	for _, c := range []*cobra.Command{assignCmd, unassignCmd} {
		c.Flags().StringVar(&assignHost, "host", "", "act on another host instead of this one")
		c.Flags().StringVar(&assignTag, "tag", "", "act on every repo carrying this tag")
		rootCmd.AddCommand(c)
	}
}

func targetHost() string {
	if assignHost != "" {
		return strings.ToLower(assignHost)
	}
	return host.Name()
}

// selectRepos resolves positional names plus an optional --tag into manifest
// indices, reporting any name that matches nothing.
func selectRepos(m manifest.Manifest, args []string) ([]int, error) {
	var idx []int
	seen := map[int]bool{}

	for _, name := range args {
		i := m.Find(name)
		if i < 0 {
			return nil, fmt.Errorf("no repo named %q in the manifest", name)
		}
		if !seen[i] {
			seen[i] = true
			idx = append(idx, i)
		}
	}

	if assignTag != "" {
		for i, r := range m.Repos {
			for _, t := range r.Tags {
				if strings.EqualFold(t, assignTag) && !seen[i] {
					seen[i] = true
					idx = append(idx, i)
				}
			}
		}
	}

	if len(idx) == 0 {
		return nil, fmt.Errorf("nothing selected — name at least one repo or pass --tag")
	}
	return idx, nil
}

func runAssign(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	idx, err := selectRepos(m, args)
	if err != nil {
		return err
	}

	target := targetHost()
	changed := 0
	for _, i := range idx {
		r := &m.Repos[i]
		if r.HasHost(target) {
			fmt.Printf("  %s %-28s already on %s\n", dim("·"), r.Name, target)
			continue
		}
		r.Hosts = append(r.Hosts, target)
		changed++
		fmt.Printf("  %s %-28s → %s\n", green("+"), r.Name, target)
	}

	if changed == 0 {
		return nil
	}
	if err := manifest.Save(m); err != nil {
		return err
	}
	fmt.Printf("\n%d repos assigned to %s — next: repoman sync-manifest && repoman clone\n", changed, target)
	return nil
}

func runUnassign(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	idx, err := selectRepos(m, args)
	if err != nil {
		return err
	}

	target := targetHost()
	changed := 0
	for _, i := range idx {
		if !m.Repos[i].HasHost(target) {
			continue
		}
		m.RemoveHost(m.Repos[i].Name, target)
		changed++
		fmt.Printf("  %s %-28s no longer on %s\n", yellow("-"), m.Repos[i].Name, target)
	}

	if changed == 0 {
		fmt.Printf("Nothing to do — none of those repos were assigned to %s\n", target)
		return nil
	}
	if err := manifest.Save(m); err != nil {
		return err
	}
	fmt.Printf("\n%d repos unassigned from %s. Local clones were not touched.\n", changed, target)
	return nil
}
