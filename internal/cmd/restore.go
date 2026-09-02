package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <repo|bundle>...",
	Short: "Clone an archived project back and restore its local state",
	Long: `Bring an archived project back: clone from its remote, restore the untracked
state restic holds (.env files, local config, certificates), and mark it active
again. For a bundle, its asset directory comes back too.

The git clone and the restic restore are separate halves of the same job —
either alone leaves you with a project that does not run.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRestore,
}

var (
	restoreDryRun bool
	restoreHost   string
)

func init() {
	restoreCmd.Flags().BoolVar(&restoreDryRun, "dry-run", false, "show what would be restored, change nothing")
	restoreCmd.Flags().StringVar(&restoreHost, "from-host", "", "restore local state from another host's snapshots")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	hostname := host.Name()
	snapshotHost := restoreHost
	if snapshotHost == "" {
		snapshotHost = hostname
	}

	var repoNames []string
	var bundles []manifest.Bundle
	for _, name := range args {
		if i := m.FindBundle(name); i >= 0 {
			bundles = append(bundles, m.Bundles[i])
			repoNames = append(repoNames, m.Bundles[i].Repos...)
			continue
		}
		if m.Find(name) < 0 {
			return fmt.Errorf("no repo or bundle named %q", name)
		}
		repoNames = append(repoNames, name)
	}

	repo, passFile, err := resticTarget(m)
	if err != nil {
		return err
	}

	var restorePaths []string
	for _, name := range repoNames {
		i := m.Find(name)
		r := m.Repos[i]
		path := m.PathFor(r, hostname)

		if _, err := os.Stat(path); err == nil {
			fmt.Printf("  %s %-32s %s\n", dim("·"), r.Name, dim("already present"))
		} else if r.Remote == "" {
			fmt.Printf("  %s %-32s %s\n", red("✗"), r.Name, red("no remote to clone from"))
		} else if restoreDryRun {
			fmt.Printf("  %s %-32s %s\n", green("+"), r.Name, dim("would clone "+r.Remote))
		} else {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := gitx.Run(filepath.Dir(path), "clone", r.Remote, filepath.Base(path)); err != nil {
				return fmt.Errorf("cloning %s: %w", r.Name, err)
			}
			fmt.Printf("  %s %-32s %s\n", green("✓"), r.Name, dim("cloned"))
		}
		restorePaths = append(restorePaths, path)
		if !restoreDryRun {
			m.Repos[i].Status = "active"
			if !m.Repos[i].HasHost(hostname) {
				m.Repos[i].Hosts = append(m.Repos[i].Hosts, hostname)
			}
		}
	}

	for _, b := range bundles {
		if dir := m.AssetsDir(b); dir != "" {
			restorePaths = append(restorePaths, dir)
			fmt.Printf("  %s %-32s %s\n", green("+"), b.Name+" (assets)", dim(manifest.Contract(dir)))
		}
		if i := m.FindBundle(b.Name); i >= 0 && !restoreDryRun {
			m.Bundles[i].Status = "active"
		}
	}

	if restoreDryRun {
		fmt.Printf("\n%s\n", dim("dry run: nothing was cloned or restored"))
		return nil
	}

	// Restore the untracked half from the newest snapshot for the host. Paths
	// are absolute in the snapshot, so the target is /.
	fmt.Printf("\nRestoring local state from %s (host %s)…\n", repo, snapshotHost)
	resticArgs := []string{"-r", repo, "restore", "latest", "--host", snapshotHost, "--target", "/"}
	for _, p := range restorePaths {
		resticArgs = append(resticArgs, "--include", p)
	}
	c := exec.Command("restic", resticArgs...)
	c.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+passFile)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Printf("  %s %s\n", yellow("!"), yellow("no local state restored: "+err.Error()))
		fmt.Printf("  %s\n", dim("the clones are in place; check 'repoman snapshots' if you expected files"))
	}

	if err := manifest.Save(m); err != nil {
		return err
	}
	fmt.Printf("\n%s restored — run 'repoman sync-manifest' to tell the other hosts\n", green("✓"))
	return nil
}

// resticTarget resolves the repository and password file the same way backup
// does: flag, then environment, then manifest.
func resticTarget(m manifest.Manifest) (repo, passFile string, err error) {
	repo = os.Getenv("RESTIC_REPOSITORY")
	if repo == "" {
		repo = m.Defaults.ResticRepo
	}
	if repo == "" {
		return "", "", fmt.Errorf("no restic repository configured")
	}
	passFile = os.Getenv("RESTIC_PASSWORD_FILE")
	if passFile == "" {
		passFile = m.Defaults.ResticPasswordFile
	}
	return repo, manifest.Expand(passFile), nil
}
