package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/klokie/repoman/internal/gitx"
	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var cloneCmd = &cobra.Command{
	Use:   "clone [url]",
	Short: "Clone a repo and register it, or clone everything missing on this host",
	Long: `With a URL, clone it into the default root and add it to the manifest.

With no arguments, clone every active repo the manifest assigns to this host
that is not present locally — the command that brings a new machine up to date
after 'repoman sync-manifest'.`,
	Args: cobra.MaximumNArgs(2),
	RunE: runClone,
}

var (
	cloneTags []string
	cloneJobs int
)

func init() {
	cloneCmd.Flags().StringSliceVar(&cloneTags, "tags", nil, "tags to record on a newly cloned repo")
	cloneCmd.Flags().IntVarP(&cloneJobs, "jobs", "j", 8, "parallel clones when cloning everything missing")
	rootCmd.AddCommand(cloneCmd)
}

func runClone(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cloneMissing()
	}
	return cloneURL(args)
}

func cloneURL(args []string) error {
	url := args[0]
	name := gitx.NameFromURL(url)
	if len(args) > 1 {
		name = args[1]
	}

	m := manifest.Manifest{Defaults: manifest.Defaults{Root: "~/src", AssetsRoot: "~/projects"}}
	if manifest.Exists() {
		loaded, err := manifest.Load()
		if err != nil {
			return err
		}
		m = loaded
	}

	dest := filepath.Join(m.RootDir(), name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}

	fmt.Printf("Cloning %s → %s\n", url, manifest.Contract(dest))
	if err := gitx.Run(m.RootDir(), "clone", url, name); err != nil {
		return err
	}

	_, _, _ = m.Upsert(manifest.Repo{
		Name:   name,
		Remote: url,
		Hosts:  []string{host.Name()},
		Tags:   cloneTags,
		Status: "active",
	})
	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	fmt.Printf("%s registered %s for %s — run 'repoman sync-manifest' to share it\n", green("✓"), name, host.Name())
	return nil
}

func cloneMissing() error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	hostname := host.Name()

	var missing []manifest.Repo
	for _, r := range m.ReposForHost(hostname) {
		if r.IsArchived() {
			continue
		}
		if _, err := os.Stat(m.PathFor(r, hostname)); err != nil {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		fmt.Printf("Nothing missing — every active repo for %s is already cloned\n", hostname)
		return nil
	}

	fmt.Printf("Cloning %d missing repos on %s\n\n", len(missing), hostname)
	results := forEachRepo(missing, cloneJobs, func(r manifest.Repo) result {
		if r.Remote == "" {
			return result{Skipped: true, Message: "no remote recorded"}
		}
		dest := m.PathFor(r, hostname)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return result{Err: err}
		}
		if err := gitx.Run(filepath.Dir(dest), "clone", r.Remote, filepath.Base(dest)); err != nil {
			return result{Err: err}
		}
		return result{Message: manifest.Contract(dest)}
	})

	ok, skipped, failed := tally(results)
	fmt.Printf("\n  %s cloned, %s skipped, %s failed\n", green(fmt.Sprint(ok)), dim(fmt.Sprint(skipped)), red(fmt.Sprint(failed)))
	if failed > 0 {
		return fmt.Errorf("%d repos failed to clone", failed)
	}
	return nil
}
