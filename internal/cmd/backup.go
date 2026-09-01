package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Snapshot the state git does not carry: .env files, local config, project assets",
	Long: `Back up the per-repo files that never reach a git remote.

Git already protects tracked code, so this deliberately does not snapshot whole
working copies. It collects the untracked and git-ignored files that a repo
cannot be rebuilt without — .env and friends, local overrides, certificates —
plus the asset directories under assets_root, and hands them to restic as one
snapshot.

A repo can name its own files in a .repoman-backup file, one glob per line.`,
	RunE: runBackup,
}

var (
	backupDryRun   bool
	backupRepo     string
	backupPassFile string
	backupTags     []string
	backupSkip     []string
)

// defaultPatterns are the files a working copy usually cannot be rebuilt
// without, matched against each path's base name unless they contain a slash.
var defaultPatterns = []string{
	".env", ".env.*", "*.local", ".envrc",
	"docker-compose.override.*", "*.pem", "*.key", "*.p12",
}

// neverBackup are directories that are either reproducible or enormous.
var neverBackup = []string{
	"node_modules", ".git", ".next", ".nuxt", "dist", "build", "coverage",
	"vendor", ".venv", "venv", "__pycache__", ".turbo", ".cache", ".nx",
	"target", "Pods", ".terraform", ".gradle", ".DS_Store",
	"cache", "audio_cache", "tmp",
}

func init() {
	backupCmd.Flags().BoolVar(&backupDryRun, "dry-run", false, "list what would be backed up without running restic")
	backupCmd.Flags().StringVar(&backupRepo, "repo", "", "restic repository (default: defaults.restic_repo)")
	backupCmd.Flags().StringVar(&backupPassFile, "password-file", "", "restic password file (default: defaults.restic_password_file)")
	backupCmd.Flags().StringSliceVar(&backupTags, "tag", nil, "extra restic tags")
	backupCmd.Flags().StringSliceVar(&backupSkip, "skip", nil, "repo name globs to leave out (e.g. 'werlabs-*')")
	rootCmd.AddCommand(backupCmd)
}

func runBackup(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	hostname := host.Name()

	// Flag, then environment, then manifest: the manifest holds the value that
	// is right for most hosts (sftp to the always-on one), and the host that
	// physically owns the disk overrides it with a local path.
	repo := backupRepo
	if repo == "" {
		repo = os.Getenv("RESTIC_REPOSITORY")
	}
	if repo == "" {
		repo = m.Defaults.ResticRepo
	}
	if repo == "" {
		return fmt.Errorf("no restic repository configured — set defaults.restic_repo in %s or pass --repo", manifest.Path())
	}
	passFile := backupPassFile
	if passFile == "" {
		passFile = os.Getenv("RESTIC_PASSWORD_FILE")
	}
	if passFile == "" {
		passFile = m.Defaults.ResticPasswordFile
	}
	if passFile == "" {
		return fmt.Errorf("no restic password file configured — set defaults.restic_password_file or pass --password-file")
	}
	passFile = manifest.Expand(passFile)
	if _, err := os.Stat(passFile); err != nil {
		return fmt.Errorf("restic password file %s: %w", passFile, err)
	}

	var paths []string
	perRepo := map[string]int{}
	skip := append([]string{}, backupSkip...)
	skip = append(skip, m.Defaults.BackupSkip...)

	for _, r := range m.ReposForHost(hostname) {
		if r.IsArchived() {
			continue
		}
		if skipped, pattern := matchesSkip(r.Name, skip); skipped {
			fmt.Printf("  %s %-36s %s\n", dim("-"), r.Name, dim("skipped by "+pattern))
			continue
		}
		dir := m.PathFor(r, hostname)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		found := collectRepoState(dir)
		if len(found) > 0 {
			perRepo[r.Name] = len(found)
			paths = append(paths, found...)
		}
	}

	// Asset directories are not git repos at all, so they go in whole.
	if root := m.Defaults.AssetsRoot; root != "" {
		assets := manifest.Expand(root)
		if entries, err := os.ReadDir(assets); err == nil {
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					paths = append(paths, filepath.Join(assets, e.Name()))
					perRepo["assets/"+e.Name()] = -1
				}
			}
		}
	}

	for _, extra := range m.Defaults.ExtraPaths {
		p := manifest.Expand(extra)
		if _, err := os.Stat(p); err != nil {
			continue // not on this host
		}
		kept, skipped := splitNestedRepos(p)
		paths = append(paths, kept...)
		perRepo[extra] = -1
		for _, s := range skipped {
			fmt.Printf("  %s %-36s %s\n", dim("·"), manifest.Contract(s),
				dim("skipped — git repo, belongs in the manifest"))
		}
	}

	if excludes := m.Defaults.BackupExclude; len(excludes) > 0 {
		var kept []string
		dropped := 0
		for _, p := range paths {
			if ok, pattern := matchesSkip(manifest.Contract(p), excludes); ok {
				fmt.Printf("  %s %-36s %s\n", dim("-"), manifest.Contract(p), dim("excluded by "+pattern))
				dropped++
				continue
			}
			kept = append(kept, p)
		}
		paths = kept
		_ = dropped
	}

	if len(paths) == 0 {
		fmt.Println("Nothing to back up — no untracked local state found")
		return nil
	}
	sort.Strings(paths)

	names := make([]string, 0, len(perRepo))
	for n := range perRepo {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if perRepo[n] < 0 {
			fmt.Printf("  %s %-36s %s\n", dim("·"), n, dim("whole directory"))
			continue
		}
		fmt.Printf("  %s %-36s %d files\n", green("+"), n, perRepo[n])
	}
	fmt.Printf("\n%d paths from %d sources → %s\n", len(paths), len(perRepo), repo)

	if backupDryRun {
		for _, p := range paths {
			fmt.Printf("    %s\n", dim(manifest.Contract(p)))
		}
		fmt.Printf("\n%s\n", dim("dry run: restic was not called"))
		return nil
	}

	list, err := os.CreateTemp("", "repoman-backup-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(list.Name())
	for _, p := range paths {
		fmt.Fprintln(list, p)
	}
	if err := list.Close(); err != nil {
		return err
	}

	resticArgs := []string{
		"backup", "--files-from-verbatim", list.Name(),
		"--tag", "repoman", "--tag", "host:" + hostname,
		// Pin the snapshot host to repoman's identity: restic would otherwise
		// record the OS hostname, which on these machines drifts to
		// "mac.lan" or "Gatekeeper" and makes `restic snapshots --host` useless.
		"--host", hostname,
	}
	for _, d := range neverBackup {
		resticArgs = append(resticArgs, "--exclude", d)
	}
	for _, t := range backupTags {
		resticArgs = append(resticArgs, "--tag", t)
	}

	c := exec.Command("restic", resticArgs...)
	c.Env = append(os.Environ(),
		"RESTIC_REPOSITORY="+repo,
		"RESTIC_PASSWORD_FILE="+passFile,
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("restic backup: %w", err)
	}
	return nil
}

// collectRepoState returns the untracked and git-ignored files in a repo that
// look like state worth keeping. Tracked files are excluded on purpose: the
// remote already has them.
func collectRepoState(dir string) []string {
	patterns := defaultPatterns
	if custom, err := os.ReadFile(filepath.Join(dir, ".repoman-backup")); err == nil {
		patterns = nil
		for _, line := range strings.Split(string(custom), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
	}

	seen := map[string]bool{}
	var out []string
	// Untracked-but-not-ignored, then ignored: .env.local is usually the latter.
	for _, args := range [][]string{
		{"ls-files", "-z", "--others", "--exclude-standard"},
		{"ls-files", "-z", "--others", "--ignored", "--exclude-standard"},
	} {
		listed, err := gitx.Output(dir, args...)
		if err != nil {
			continue
		}
		for _, rel := range strings.Split(listed, "\x00") {
			if rel == "" || seen[rel] || underExcludedDir(rel) || !matchesAny(rel, patterns) {
				continue
			}
			seen[rel] = true
			out = append(out, filepath.Join(dir, rel))
		}
	}
	return out
}

// splitNestedRepos expands a directory into the children worth snapshotting,
// leaving out any child that is itself a git repo — git already carries those,
// and they drag in node_modules and virtualenvs besides.
func splitNestedRepos(dir string) (keep, skipped []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{dir}, nil
	}
	nested := false
	for _, e := range entries {
		child := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(child, ".git")); err == nil {
				skipped = append(skipped, child)
				nested = true
				continue
			}
		}
		keep = append(keep, child)
	}
	if !nested {
		return []string{dir}, nil // nothing to carve out; back it up whole
	}
	return keep, skipped
}

// matchesSkip reports whether a name matches any glob. Patterns ending in /*
// also match everything deeper, which filepath.Match does not do on its own.
func matchesSkip(name string, patterns []string) (bool, string) {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true, p
		}
		if strings.HasSuffix(p, "/*") && strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
			return true, p
		}
	}
	return false, ""
}

func underExcludedDir(rel string) bool {
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		for _, skip := range neverBackup {
			if part == skip {
				return true
			}
		}
	}
	return false
}

func matchesAny(rel string, patterns []string) bool {
	base := filepath.Base(rel)
	for _, p := range patterns {
		target := base
		if strings.Contains(p, "/") {
			target = rel
		}
		if ok, _ := filepath.Match(p, target); ok {
			return true
		}
	}
	return false
}
