package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Copy snapshots from the primary restic repo to the mirror",
	Long: `Copy every snapshot the mirror is missing.

The mirror is typically a drive that is only attached to one machine — here,
Emperor on gatekeeper, which an existing Arq plan already carries offsite. When
the mirror is not present this exits quietly with success, so it can sit in a
scheduled job on every host without failing on the ones that cannot see it.`,
	RunE: runMirror,
}

var mirrorTo string

func init() {
	mirrorCmd.Flags().StringVar(&mirrorTo, "to", "", "mirror repository (default: defaults.restic_mirror)")
	rootCmd.AddCommand(mirrorCmd)
}

func runMirror(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return err
	}

	dest := mirrorTo
	if dest == "" {
		dest = m.Defaults.ResticMirror
	}
	if dest == "" {
		return fmt.Errorf("no mirror configured — set defaults.restic_mirror or pass --to")
	}
	dest = manifest.Expand(dest)

	source := os.Getenv("RESTIC_REPOSITORY")
	if source == "" {
		source = m.Defaults.ResticRepo
	}
	if source == "" {
		return fmt.Errorf("no primary repository configured")
	}

	passFile := os.Getenv("RESTIC_PASSWORD_FILE")
	if passFile == "" {
		passFile = m.Defaults.ResticPasswordFile
	}
	passFile = manifest.Expand(passFile)

	// A local mirror path that is not mounted is the normal case on the hosts
	// that do not own the drive, not an error worth failing a nightly job over.
	if !strings.Contains(dest, ":") {
		if _, err := os.Stat(dest); err != nil {
			fmt.Printf("%s mirror %s is not present — nothing to do\n", dim("·"), manifest.Contract(dest))
			return nil
		}
	}

	fmt.Printf("Mirroring %s → %s\n", source, manifest.Contract(dest))
	c := exec.Command("restic", "-r", dest, "copy",
		"--from-repo", source, "--from-password-file", passFile)
	c.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+passFile)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("restic copy: %w", err)
	}
	return nil
}
