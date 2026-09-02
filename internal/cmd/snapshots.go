package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var snapshotsCmd = &cobra.Command{
	Use:   "snapshots",
	Short: "Show the latest backup per host and flag stale ones",
	Long: `Report the most recent snapshot for every host in the manifest.

Exits non-zero when a host's newest snapshot is older than --max-age, or when
the repository cannot be reached at all — so this works as the check that turns
a silently broken backup into a visible one. A backup nobody verifies is a
backup that has already failed.`,
	RunE: runSnapshots,
}

var (
	snapMaxAge time.Duration
	snapRepo   string
	snapMirror bool
)

func init() {
	snapshotsCmd.Flags().DurationVar(&snapMaxAge, "max-age", 48*time.Hour, "flag hosts whose newest snapshot is older than this")
	snapshotsCmd.Flags().StringVar(&snapRepo, "repo", "", "repository to inspect (default: the primary)")
	snapshotsCmd.Flags().BoolVar(&snapMirror, "mirror", false, "inspect the mirror instead of the primary")
	rootCmd.AddCommand(snapshotsCmd)
}

type resticSnapshot struct {
	ID       string    `json:"short_id"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Tags     []string  `json:"tags"`
}

func runSnapshots(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}

	repo := snapRepo
	switch {
	case repo != "":
	case snapMirror:
		repo = manifest.Expand(m.Defaults.ResticMirror)
	default:
		repo = os.Getenv("RESTIC_REPOSITORY")
		if repo == "" {
			repo = m.Defaults.ResticRepo
		}
	}
	if repo == "" {
		return fmt.Errorf("no repository to inspect")
	}
	passFile := os.Getenv("RESTIC_PASSWORD_FILE")
	if passFile == "" {
		passFile = m.Defaults.ResticPasswordFile
	}

	c := exec.Command("restic", "-r", repo, "snapshots", "--json")
	c.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+manifest.Expand(passFile))
	out, err := c.Output()
	if err != nil {
		// Unreachable is itself the alert: an unmounted drive or a dead SSH
		// session looks exactly like "no backups are happening".
		return fmt.Errorf("cannot read %s — backups may not be running: %w", repo, err)
	}

	var snaps []resticSnapshot
	if err := json.Unmarshal(out, &snaps); err != nil {
		return fmt.Errorf("parsing restic output: %w", err)
	}

	// A snapshot is attributed by its host: tag, which repoman controls, rather
	// than by hostname, which the OS keeps rewriting.
	latest := map[string]resticSnapshot{}
	for _, s := range snaps {
		host := s.Hostname
		for _, t := range s.Tags {
			if rest, ok := strings.CutPrefix(t, "host:"); ok {
				host = rest
			}
		}
		if cur, ok := latest[host]; !ok || s.Time.After(cur.Time) {
			latest[host] = s
		}
	}

	hosts := m.Hosts()
	for h := range latest {
		if !contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	sort.Strings(hosts)

	fmt.Printf("  %-14s %-10s %-22s %s\n", dim("HOST"), dim("SNAPSHOT"), dim("LATEST"), dim("AGE"))
	stale := 0
	for _, h := range hosts {
		s, ok := latest[h]
		if !ok {
			fmt.Printf("  %-14s %-10s %-22s %s\n", h, dim("—"), dim("—"), red("never backed up"))
			stale++
			continue
		}
		age := time.Since(s.Time)
		ageStr := formatAge(age)
		if age > snapMaxAge {
			fmt.Printf("  %-14s %-10s %-22s %s\n", h, s.ID, s.Time.Format("2006-01-02 15:04"), red(ageStr+" — STALE"))
			stale++
			continue
		}
		fmt.Printf("  %-14s %-10s %-22s %s\n", h, s.ID, s.Time.Format("2006-01-02 15:04"), green(ageStr))
	}

	fmt.Printf("\n  %d snapshots in %s\n", len(snaps), repo)
	if stale > 0 {
		return fmt.Errorf("%d host(s) have no recent backup (threshold %s)", stale, snapMaxAge)
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
