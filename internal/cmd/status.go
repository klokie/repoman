package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show branch, dirty state, and unpushed commits for this host's repos",
	RunE:  runStatus,
}

var (
	statusHost     string
	statusProblems bool
	statusArchived bool
)

func init() {
	statusCmd.Flags().StringVar(&statusHost, "host", "", "show another host's repos instead of this one")
	statusCmd.Flags().BoolVar(&statusProblems, "problems", false, "only show repos that are dirty, unpushed, or missing")
	statusCmd.Flags().BoolVar(&statusArchived, "archived", false, "include archived repos")
	rootCmd.AddCommand(statusCmd)
}

type repoStatus struct {
	Name     string
	Path     string
	Cloned   bool
	Branch   string
	Dirty    bool
	Unpushed int
	Status   string
}

func (s repoStatus) needsAttention() bool {
	return s.Dirty || s.Unpushed > 0 || (!s.Cloned && s.Status != "archived")
}

func runStatus(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	hostname := host.Name()
	remote := false
	if statusHost != "" {
		hostname = strings.ToLower(statusHost)
		remote = hostname != host.Name()
	}

	repos := m.ReposForHost(hostname)
	if len(repos) == 0 {
		fmt.Printf("No repos configured for host %q\n", hostname)
		return nil
	}

	statuses := make([]repoStatus, len(repos))
	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(i int, repo manifest.Repo) {
			defer wg.Done()
			statuses[i] = getRepoStatus(m, repo, remote)
		}(i, repo)
	}
	wg.Wait()

	shown := make([]repoStatus, 0, len(statuses))
	for _, s := range statuses {
		if s.Status == "archived" && !statusArchived {
			continue
		}
		if statusProblems && !s.needsAttention() {
			continue
		}
		shown = append(shown, s)
	}

	if remote {
		fmt.Printf("%s (from the manifest — not inspected locally)\n\n", dim(hostname))
	}
	printStatusTable(shown, statuses, remote)
	return nil
}

// getRepoStatus inspects the working copy. For another host's repos there is
// nothing local to inspect, so only the manifest's view is reported.
func getRepoStatus(m manifest.Manifest, repo manifest.Repo, remote bool) repoStatus {
	s := repoStatus{
		Name:   repo.Name,
		Path:   m.PathFor(repo),
		Status: repo.Status,
	}
	if remote {
		return s
	}

	info, err := os.Stat(s.Path)
	if err != nil || !info.IsDir() {
		return s
	}
	s.Cloned = true
	s.Branch = gitx.CurrentBranch(s.Path)
	s.Dirty = gitx.IsDirty(s.Path)

	if gitx.HasUpstream(s.Path) {
		if out, err := gitx.Output(s.Path, "rev-list", "--count", "@{u}..HEAD"); err == nil {
			fmt.Sscanf(out, "%d", &s.Unpushed)
		}
	}
	return s
}

func printStatusTable(shown, all []repoStatus, remote bool) {
	nameW, branchW := 10, 6
	for _, s := range shown {
		if len(s.Name) > nameW {
			nameW = len(s.Name)
		}
		if len(s.Branch) > branchW {
			branchW = len(s.Branch)
		}
	}

	header := fmt.Sprintf("  %-*s  %-*s  %-9s  %s", nameW, "REPO", branchW, "BRANCH", "STATUS", "UNPUSHED")
	fmt.Println(dim(header))
	fmt.Println(dim(strings.Repeat("─", len(header)+2)))

	for _, s := range shown {
		var state, branch string
		switch {
		case s.Status == "archived":
			state, branch = dim("archived"), dim("—")
		case remote:
			state, branch = dim("—"), dim("—")
		case !s.Cloned:
			state, branch = red("missing"), dim("—")
		case s.Dirty:
			state, branch = yellow("dirty"), s.Branch
		default:
			state, branch = green("clean"), s.Branch
		}

		unpushed := dim("0")
		if s.Unpushed > 0 {
			unpushed = yellow(fmt.Sprintf("%d", s.Unpushed))
		}
		if !s.Cloned || s.Status == "archived" {
			unpushed = dim("—")
		}
		fmt.Printf("  %-*s  %-*s  %-9s  %s\n", nameW, s.Name, branchW, branch, state, unpushed)
	}

	var cloned, missing, archived, dirty, unpushed int
	for _, s := range all {
		switch {
		case s.Status == "archived":
			archived++
		case !s.Cloned:
			missing++
		default:
			cloned++
			if s.Dirty {
				dirty++
			}
			if s.Unpushed > 0 {
				unpushed++
			}
		}
	}

	fmt.Println()
	if remote {
		fmt.Printf("  %d repos: %d active, %s archived\n", len(all), len(all)-archived, dim(fmt.Sprint(archived)))
		return
	}
	fmt.Printf("  %d repos: %s cloned, %s dirty, %s unpushed, %s missing, %s archived\n",
		len(all),
		green(fmt.Sprint(cloned)), yellow(fmt.Sprint(dirty)), yellow(fmt.Sprint(unpushed)),
		red(fmt.Sprint(missing)), dim(fmt.Sprint(archived)))
}
