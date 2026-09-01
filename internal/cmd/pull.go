package cmd

import (
	"fmt"
	"os"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull every active repo on this host, in parallel",
	Long: `Fast-forward all active repos assigned to this host.

Dirty repos and repos whose branch has no upstream are skipped rather than
risking a merge you did not ask for; --rebase --autostash is available for the
cases where you want them pulled anyway.`,
	RunE: runPull,
}

var (
	pullJobs      int
	pullRebase    bool
	pullAutostash bool
)

func init() {
	pullCmd.Flags().IntVarP(&pullJobs, "jobs", "j", 8, "parallel git pulls")
	pullCmd.Flags().BoolVar(&pullRebase, "rebase", false, "pull with --rebase instead of fast-forward-only")
	pullCmd.Flags().BoolVar(&pullAutostash, "autostash", false, "stash local changes across the pull (implies --rebase)")
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	hostname := host.Name()

	var repos []manifest.Repo
	for _, r := range m.ReposForHost(hostname) {
		if !r.IsArchived() {
			repos = append(repos, r)
		}
	}
	if len(repos) == 0 {
		fmt.Printf("No active repos for host %q\n", hostname)
		return nil
	}

	fmt.Printf("Pulling %d repos on %s\n\n", len(repos), hostname)
	results := forEachRepo(repos, pullJobs, func(r manifest.Repo) result {
		path := m.PathFor(r)
		if _, err := os.Stat(path); err != nil {
			return result{Skipped: true, Message: "not cloned — run 'repoman clone'"}
		}
		if !gitx.HasUpstream(path) {
			return result{Skipped: true, Message: "no upstream branch"}
		}
		if gitx.IsDirty(path) && !pullAutostash {
			return result{Skipped: true, Message: "dirty working tree"}
		}

		gitArgs := []string{"pull", "--ff-only"}
		if pullRebase || pullAutostash {
			gitArgs = []string{"pull", "--rebase"}
		}
		if pullAutostash {
			gitArgs = append(gitArgs, "--autostash")
		}

		before, _ := gitx.Output(path, "rev-parse", "HEAD")
		if err := gitx.Run(path, gitArgs...); err != nil {
			return result{Err: err}
		}
		after, _ := gitx.Output(path, "rev-parse", "HEAD")
		if before == after {
			return result{Message: dim("up to date")}
		}
		count, _ := gitx.Output(path, "rev-list", "--count", before+".."+after)
		return result{Message: fmt.Sprintf("%s new commits (%s)", count, gitx.CurrentBranch(path))}
	})

	ok, skipped, failed := tally(results)
	fmt.Printf("\n  %s pulled, %s skipped, %s failed\n", green(fmt.Sprint(ok)), dim(fmt.Sprint(skipped)), red(fmt.Sprint(failed)))
	if failed > 0 {
		return fmt.Errorf("%d repos failed to pull", failed)
	}
	return nil
}
