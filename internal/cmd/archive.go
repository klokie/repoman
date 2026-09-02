package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var archiveCmd = &cobra.Command{
	Use:   "archive <repo|bundle>...",
	Short: "Back up local state, remove the clone, and mark it archived",
	Long: `Shelve a project: snapshot what git does not carry, delete the local clone,
and record it as archived so it can be restored months later.

This refuses to run on a repo with uncommitted changes, unpushed commits, or no
remote — deleting a clone is only safe when the remote genuinely has everything.
Use --force to override, but understand that --force on a repo with no remote
destroys the only copy.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runArchive,
}

var (
	archiveForce  bool
	archiveDryRun bool
	archiveYes    bool
)

func init() {
	archiveCmd.Flags().BoolVar(&archiveForce, "force", false, "archive even when the remote does not have everything")
	archiveCmd.Flags().BoolVar(&archiveDryRun, "dry-run", false, "show what would happen, delete nothing")
	archiveCmd.Flags().BoolVarP(&archiveYes, "yes", "y", false, "do not ask before deleting")
	rootCmd.AddCommand(archiveCmd)
}

// safetyCheck reports why a repo is not safe to delete locally, or "" if it is.
func safetyCheck(m manifest.Manifest, r manifest.Repo, path string) string {
	if r.Remote == "" {
		return "no remote — the local clone is the only copy"
	}
	if _, err := os.Stat(path); err != nil {
		return "" // already gone; nothing to lose
	}
	if gitx.IsDirty(path) {
		return "uncommitted changes"
	}
	if !gitx.HasUpstream(path) {
		return "branch has no upstream"
	}
	if out, err := gitx.Output(path, "rev-list", "--count", "@{u}..HEAD"); err == nil && out != "0" {
		return out + " unpushed commit(s)"
	}
	if out, err := gitx.Output(path, "stash", "list"); err == nil && out != "" {
		return fmt.Sprintf("%d stash entr(ies)", len(strings.Split(out, "\n")))
	}
	return ""
}

func runArchive(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	hostname := host.Name()

	// Expand bundle names into their repos, keeping track of asset dirs.
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

	type target struct {
		repo manifest.Repo
		path string
		why  string
	}
	var targets []target
	blocked := 0
	for _, name := range repoNames {
		i := m.Find(name)
		if i < 0 {
			return fmt.Errorf("bundle references unknown repo %q", name)
		}
		r := m.Repos[i]
		path := m.PathFor(r, hostname)
		why := safetyCheck(m, r, path)
		targets = append(targets, target{r, path, why})
		if why != "" {
			blocked++
		}
	}

	for _, t := range targets {
		switch {
		case t.why != "" && !archiveForce:
			fmt.Printf("  %s %-32s %s\n", red("✗"), t.repo.Name, red(t.why))
		case t.why != "":
			fmt.Printf("  %s %-32s %s\n", yellow("!"), t.repo.Name, yellow(t.why+" — forced"))
		default:
			fmt.Printf("  %s %-32s %s\n", green("✓"), t.repo.Name, dim("safe: remote has everything"))
		}
	}
	for _, b := range bundles {
		if dir := m.AssetsDir(b); dir != "" {
			fmt.Printf("  %s %-32s %s\n", green("✓"), b.Name+" (assets)", dim(manifest.Contract(dir)))
		}
	}

	if blocked > 0 && !archiveForce {
		return fmt.Errorf("%d repo(s) are not safe to archive — resolve them, or pass --force", blocked)
	}
	if archiveDryRun {
		fmt.Printf("\n%s\n", dim("dry run: nothing was backed up or deleted"))
		return nil
	}

	// Back up before deleting anything: the whole point of archiving is that
	// the untracked state survives the clone going away.
	fmt.Printf("\nBacking up local state first…\n")
	backupDryRun = false
	if err := runBackup(cmd, nil); err != nil {
		return fmt.Errorf("refusing to delete anything — backup failed: %w", err)
	}

	// Verify the backup actually holds what is about to be deleted. An exclude
	// pattern that matches more than intended is invisible in the backup output
	// — and deleting on the strength of a snapshot nobody checked is how an
	// archive command destroys data.
	var assetDirs []string
	for _, b := range bundles {
		if dir := m.AssetsDir(b); dir != "" {
			assetDirs = append(assetDirs, dir)
		}
	}
	if err := verifyBackedUp(m, hostname, assetDirs); err != nil {
		return fmt.Errorf("refusing to delete anything — %w", err)
	}

	if !archiveYes && !confirm(fmt.Sprintf("\nDelete %d local clone(s)?", len(targets))) {
		return fmt.Errorf("aborted")
	}

	for _, t := range targets {
		if _, err := os.Stat(t.path); err == nil {
			if err := removeTree(t.path); err != nil {
				return fmt.Errorf("removing %s: %w", t.path, err)
			}
			fmt.Printf("  %s removed %s\n", yellow("-"), manifest.Contract(t.path))
		}
		if i := m.Find(t.repo.Name); i >= 0 {
			m.Repos[i].Status = "archived"
		}
	}
	for _, b := range bundles {
		if i := m.FindBundle(b.Name); i >= 0 {
			m.Bundles[i].Status = "archived"
		}
		if dir := m.AssetsDir(b); dir != "" {
			if _, err := os.Stat(dir); err == nil {
				if err := removeTree(dir); err != nil {
					return fmt.Errorf("removing %s: %w", dir, err)
				}
				fmt.Printf("  %s removed %s\n", yellow("-"), manifest.Contract(dir))
			}
		}
	}

	if err := manifest.Save(m); err != nil {
		return err
	}
	fmt.Printf("\n%s archived — run 'repoman sync-manifest' to tell the other hosts\n", green("✓"))
	return nil
}

// verifyBackedUp confirms each asset directory is present in the newest
// snapshot with at least one file under it, not merely as an empty directory
// entry.
func verifyBackedUp(m manifest.Manifest, hostname string, dirs []string) error {
	if len(dirs) == 0 {
		return nil
	}
	repo, passFile, err := resticTarget(m)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		c := exec.Command("restic", "-r", repo, "ls", "latest", "--host", hostname, dir)
		c.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+passFile)
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("could not verify %s in the backup: %w", manifest.Contract(dir), err)
		}
		files := 0
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "snapshot ") || line == dir {
				continue
			}
			files++
		}
		if files == 0 {
			return fmt.Errorf("%s is in the snapshot but empty — an exclude pattern is swallowing its contents",
				manifest.Contract(dir))
		}
		fmt.Printf("  %s %-32s %s\n", green("✓"), manifest.Contract(dir), dim(fmt.Sprintf("verified: %d entries in the backup", files)))
	}
	return nil
}

// removeTree deletes a directory, clearing the immutable flags and read-only
// bits that legacy Drupal and WordPress trees are full of.
func removeTree(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	exec.Command("chflags", "-R", "nouchg", path).Run()
	exec.Command("chmod", "-R", "u+w", path).Run()
	return os.RemoveAll(path)
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(line), "y")
}
