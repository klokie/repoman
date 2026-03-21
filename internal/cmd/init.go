package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scan existing repos and generate initial manifest",
	Long:  "Scan ~/Sites and ~/src for git repos and create a manifest.toml with all discovered repositories.",
	RunE:  runInit,
}

var initRoots []string

func init() {
	initCmd.Flags().StringSliceVar(&initRoots, "roots", nil, "directories to scan (default: ~/Sites, ~/src)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if manifest.Exists() {
		return fmt.Errorf("manifest already exists at %s — use 'repoman status' or edit manually", manifest.Path())
	}

	roots := initRoots
	if len(roots) == 0 {
		home, _ := os.UserHomeDir()
		roots = []string{
			filepath.Join(home, "Sites"),
			filepath.Join(home, "src"),
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}
	hostname = strings.Split(hostname, ".")[0]

	var repos []manifest.Repo
	for _, root := range roots {
		found, err := scanDir(root, hostname)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", root, err)
			continue
		}
		repos = append(repos, found...)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no git repos found in %v", roots)
	}

	m := manifest.Manifest{
		Defaults: manifest.Defaults{
			Root: "~/src",
		},
		Repos: repos,
	}

	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("Created %s with %d repos\n", manifest.Path(), len(repos))
	return nil
}

func scanDir(root string, hostname string) ([]manifest.Repo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var repos []manifest.Repo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(root, entry.Name())
		gitDir := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitDir); err != nil {
			continue
		}

		remote := getRemoteURL(repoPath)

		repos = append(repos, manifest.Repo{
			Name:   entry.Name(),
			Remote: remote,
			Path:   strings.Replace(repoPath, os.Getenv("HOME"), "~", 1),
			Hosts:  []string{hostname},
			Status: "active",
		})
	}

	return repos, nil
}

func getRemoteURL(repoPath string) string {
	c := exec.Command("git", "remote", "get-url", "origin")
	c.Dir = repoPath
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
