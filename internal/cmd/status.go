package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all repos in the manifest",
	RunE:  runStatus,
}

func init() {
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

func runStatus(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}
	hostname = strings.Split(hostname, ".")[0]

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
			statuses[i] = getRepoStatus(repo)
		}(i, repo)
	}
	wg.Wait()

	printStatusTable(statuses)
	return nil
}

func getRepoStatus(repo manifest.Repo) repoStatus {
	s := repoStatus{
		Name:   repo.Name,
		Path:   repo.ExpandedPath(),
		Status: repo.Status,
	}

	info, err := os.Stat(s.Path)
	if err != nil || !info.IsDir() {
		s.Cloned = false
		return s
	}
	s.Cloned = true

	if branch, err := gitOutput(s.Path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		s.Branch = strings.TrimSpace(branch)
	}

	if status, err := gitOutput(s.Path, "status", "--porcelain"); err == nil {
		s.Dirty = strings.TrimSpace(status) != ""
	}

	if log, err := gitOutput(s.Path, "log", "--oneline", "@{u}..HEAD"); err == nil {
		trimmed := strings.TrimSpace(log)
		if trimmed != "" {
			s.Unpushed = len(strings.Split(trimmed, "\n"))
		}
	}

	return s
}

func gitOutput(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.Output()
	return string(out), err
}

func printStatusTable(statuses []repoStatus) {
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	nameW, branchW := 10, 6
	for _, s := range statuses {
		if len(s.Name) > nameW {
			nameW = len(s.Name)
		}
		if len(s.Branch) > branchW {
			branchW = len(s.Branch)
		}
	}

	header := fmt.Sprintf("  %-*s  %-*s  %-9s  %s", nameW, "REPO", branchW, "BRANCH", "STATUS", "UNPUSHED")
	fmt.Println(dimStyle.Render(header))
	fmt.Println(dimStyle.Render(strings.Repeat("─", len(header)+2)))

	for _, s := range statuses {
		var state, branch string

		switch {
		case s.Status == "archived":
			state = dimStyle.Render("archived")
			branch = dimStyle.Render("—")
		case !s.Cloned:
			state = redStyle.Render("missing")
			branch = dimStyle.Render("—")
		case s.Dirty:
			state = yellowStyle.Render("dirty")
			branch = s.Branch
		default:
			state = greenStyle.Render("clean")
			branch = s.Branch
		}

		unpushed := dimStyle.Render("0")
		if s.Unpushed > 0 {
			unpushed = yellowStyle.Render(fmt.Sprintf("%d", s.Unpushed))
		}
		if !s.Cloned || s.Status == "archived" {
			unpushed = dimStyle.Render("—")
		}

		fmt.Printf("  %-*s  %-*s  %-9s  %s\n", nameW, s.Name, branchW, branch, state, unpushed)
	}

	var cloned, missing, archived, dirty int
	for _, s := range statuses {
		switch {
		case s.Status == "archived":
			archived++
		case !s.Cloned:
			missing++
		case s.Dirty:
			dirty++
			cloned++
		default:
			cloned++
		}
	}

	fmt.Println()
	fmt.Printf("  %d repos: %s cloned, %s dirty, %s missing, %s archived\n",
		len(statuses),
		greenStyle.Render(fmt.Sprintf("%d", cloned)),
		yellowStyle.Render(fmt.Sprintf("%d", dirty)),
		redStyle.Render(fmt.Sprintf("%d", missing)),
		dimStyle.Render(fmt.Sprintf("%d", archived)),
	)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
