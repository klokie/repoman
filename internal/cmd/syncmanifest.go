package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var syncManifestCmd = &cobra.Command{
	Use:   "sync-manifest",
	Short: "Pull, commit, and push the manifest so every host agrees",
	Long: `Keep ~/.config/repoman in sync across machines via a git remote.

Run it with --init <remote> once per machine to attach the config directory to
a (private) git repo; after that, plain 'repoman sync-manifest' rebases onto
the remote, commits any local manifest changes, and pushes them.`,
	RunE: runSyncManifest,
}

var syncInitRemote string

func init() {
	syncManifestCmd.Flags().StringVar(&syncInitRemote, "init", "", "attach the config dir to this git remote (first run on a machine)")
	rootCmd.AddCommand(syncManifestCmd)
}

func runSyncManifest(cmd *cobra.Command, args []string) error {
	dir := manifest.Dir()
	if syncInitRemote != "" {
		return initManifestRepo(dir, syncInitRemote)
	}

	if !gitx.IsRepo(dir) {
		return fmt.Errorf("%s is not a git repo — run 'repoman sync-manifest --init <remote>' first", dir)
	}

	fmt.Printf("Syncing %s\n", dir)

	// Commit before pulling. --autostash would apply local edits on top of the
	// rebased file and can leave conflict markers behind while still exiting 0,
	// which is how an unparseable manifest once reached every host. Committing
	// first turns the same situation into an ordinary rebase git can either
	// merge or halt on.
	if gitx.IsDirty(dir) {
		if err := gitx.Run(dir, "add", "-A"); err != nil {
			return err
		}
		msg := fmt.Sprintf("manifest: update from %s (%s)", host.Name(), time.Now().Format("2006-01-02"))
		if err := gitx.Run(dir, "commit", "-m", msg); err != nil {
			return err
		}
		fmt.Printf("  %s committed local changes\n", green("✓"))
	} else {
		fmt.Printf("  %s no local changes\n", dim("·"))
	}

	if err := gitx.Run(dir, "pull", "--rebase"); err != nil {
		if abortErr := gitx.Run(dir, "rebase", "--abort"); abortErr == nil {
			return fmt.Errorf("the manifest changed on another host in a way git cannot merge — "+
				"resolve it in %s by hand, then re-run: %w", dir, err)
		}
		return fmt.Errorf("pulling manifest: %w", err)
	}

	// Belt and braces: never push something no host can parse.
	if _, err := manifest.Load(); err != nil {
		return fmt.Errorf("manifest is not parseable after the pull — resolve it by hand, then re-run: %w", err)
	}

	if err := gitx.Run(dir, "push"); err != nil {
		return fmt.Errorf("pushing manifest: %w", err)
	}
	fmt.Printf("  %s pushed — other hosts get it with 'repoman sync-manifest'\n", green("✓"))
	return nil
}

// appendExclude adds a pattern to .git/info/exclude (local-only ignores).
func appendExclude(dir, pattern string) error {
	p := filepath.Join(dir, ".git", "info", "exclude")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n%s\n", pattern)
	return err
}

// initManifestRepo attaches the config dir to a remote. It never clobbers an
// existing local manifest: if both sides have one, the local copy is set aside
// as manifest.local.toml for a manual merge.
func initManifestRepo(dir, remote string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if gitx.IsRepo(dir) {
		return fmt.Errorf("%s is already a git repo — plain 'repoman sync-manifest' is what you want", dir)
	}

	if err := gitx.Run(dir, "init", "-b", "main"); err != nil {
		return err
	}
	if err := gitx.Run(dir, "remote", "add", "origin", remote); err != nil {
		return err
	}

	if err := appendExclude(dir, "host"); err != nil {
		return err
	}

	remoteHasMain := gitx.Run(dir, "fetch", "origin", "main") == nil
	localHasManifest := manifest.Exists()

	switch {
	case remoteHasMain && localHasManifest:
		backup := filepath.Join(dir, "manifest.local.toml")
		if err := os.Rename(manifest.Path(), backup); err != nil {
			return err
		}
		if err := gitx.Run(dir, "checkout", "-B", "main", "origin/main"); err != nil {
			return err
		}
		// Keep the set-aside copy out of the shared repo without committing a
		// .gitignore that every other host would then carry.
		if err := appendExclude(dir, "manifest.local.toml"); err != nil {
			return err
		}
		fmt.Printf("%s pulled the shared manifest; your previous local copy is at %s\n", yellow("!"), backup)
		fmt.Printf("  Merge it with: repoman init   (re-scans this host into the shared manifest)\n")
		return nil

	case remoteHasMain:
		if err := gitx.Run(dir, "checkout", "-B", "main", "origin/main"); err != nil {
			return err
		}
		fmt.Printf("%s cloned the shared manifest into %s\n", green("✓"), dir)
		fmt.Printf("  Next: repoman init && repoman clone\n")
		return nil

	default:
		if !localHasManifest {
			return fmt.Errorf("remote %s is empty and there is no local manifest — run 'repoman init' first", remote)
		}
		if err := gitx.Run(dir, "add", "-A"); err != nil {
			return err
		}
		if err := gitx.Run(dir, "commit", "-m", fmt.Sprintf("manifest: seed from %s", host.Name())); err != nil {
			return err
		}
		if err := gitx.Run(dir, "push", "-u", "origin", "main"); err != nil {
			return err
		}
		fmt.Printf("%s seeded %s from %s\n", green("✓"), remote, host.Name())
		return nil
	}
}
