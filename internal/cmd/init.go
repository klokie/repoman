package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scan local repos and add them to the manifest",
	Long: `Scan for git repos and record them in the manifest under this host's name.

Safe to re-run, and safe to run on a second machine against a manifest that
already describes the first: repos already listed just gain this host, they are
never duplicated or overwritten.`,
	RunE: runInit,
}

var (
	initRoots  []string
	initDryRun bool
)

func init() {
	initCmd.Flags().StringSliceVar(&initRoots, "roots", nil, "directories to scan (default: ~/src, ~/Sites)")
	initCmd.Flags().BoolVar(&initDryRun, "dry-run", false, "show what would change without writing the manifest")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	m := manifest.Manifest{Defaults: manifest.Defaults{Root: "~/src", AssetsRoot: "~/projects"}}
	if manifest.Exists() {
		loaded, err := manifest.Load()
		if err != nil {
			return err
		}
		m = loaded
	}

	roots := initRoots
	if len(roots) == 0 {
		home, _ := os.UserHomeDir()
		roots = []string{filepath.Join(home, "src"), filepath.Join(home, "Sites")}
	}

	hostname := host.Name()
	var added, joined int

	for _, root := range roots {
		found, err := scanDir(root, hostname)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", root, err)
			}
			continue
		}
		for _, repo := range found {
			// Drop the path when it is just <root>/<name>, so the entry stays
			// portable to a host whose default root differs.
			if repo.ExpandedPath() == filepath.Join(m.RootDir(), repo.Name) {
				repo.Path = ""
			}
			isNew, gainedHost := m.Upsert(repo)
			switch {
			case isNew:
				added++
				fmt.Printf("  %s %s\n", green("+"), repo.Name)
			case gainedHost:
				joined++
				fmt.Printf("  %s %s (now also on %s)\n", yellow("~"), repo.Name, hostname)
			}
		}
	}

	if added == 0 && joined == 0 {
		fmt.Printf("Nothing new — manifest already describes every repo found on %s\n", hostname)
		return nil
	}

	if initDryRun {
		fmt.Printf("\ndry run: %d new, %d newly on %s (manifest unchanged)\n", added, joined, hostname)
		return nil
	}

	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	fmt.Printf("\n%s: %d new, %d newly on %s, %d total\n", manifest.Path(), added, joined, hostname, len(m.Repos))
	return nil
}

// scanDir finds git repos one level below root. A repo named _archived is not a
// repo but a holding pen, so it is descended into and its contents marked
// archived.
func scanDir(root string, hostname string) ([]manifest.Repo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var repos []manifest.Repo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		repoPath := filepath.Join(root, entry.Name())

		if entry.Name() == "_archived" {
			archived, err := scanDir(repoPath, hostname)
			if err != nil {
				continue
			}
			for i := range archived {
				archived[i].Status = "archived"
			}
			repos = append(repos, archived...)
			continue
		}

		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
			continue
		}

		repos = append(repos, manifest.Repo{
			Name:   entry.Name(),
			Remote: gitx.RemoteURL(repoPath),
			Path:   manifest.Contract(repoPath),
			Hosts:  []string{hostname},
			Status: "active",
		})
	}

	return repos, nil
}
