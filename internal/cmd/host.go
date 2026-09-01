package cmd

import (
	"fmt"

	"github.com/klokie/repoman/internal/host"
	"github.com/klokie/repoman/internal/manifest"
	"github.com/spf13/cobra"
)

var hostCmd = &cobra.Command{
	Use:   "host [name]",
	Short: "Show or pin this machine's name in the manifest",
	Long: `Without arguments, print the name repoman uses for this machine.

With a name, pin it. Pin the name whenever the OS hostname is unreliable —
macOS appends a suffix ("oleander-5") after a Bonjour collision, which would
otherwise split one machine's repos across two identities in the manifest.
The pin is local to this machine and is not shared through the manifest repo.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHost,
}

var hostFrom string

func init() {
	hostCmd.Flags().StringVar(&hostFrom, "from", "", "also rewrite manifest entries filed under this old host name")
	rootCmd.AddCommand(hostCmd)
}

// renameHostInManifest moves every entry from an old host name to a new one,
// which is what a Bonjour rename ("oleander" -> "oleander-5") leaves behind.
func renameHostInManifest(from, to string) (int, error) {
	m, err := manifest.Load()
	if err != nil {
		return 0, err
	}
	moved := 0
	for i := range m.Repos {
		r := &m.Repos[i]
		if !r.HasHost(from) {
			continue
		}
		m.RemoveHost(r.Name, from)
		if !r.HasHost(to) {
			r.Hosts = append(r.Hosts, to)
		}
		moved++
	}
	if moved == 0 {
		return 0, nil
	}
	return moved, manifest.Save(m)
}

func runHost(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		name := host.Name()
		fmt.Println(name)
		if detected := host.Detected(); detected != name {
			fmt.Printf("  %s pinned in %s (OS reports %q)\n", dim("·"), host.HostFile(), detected)
		}
		return nil
	}

	if err := host.Set(args[0]); err != nil {
		return err
	}
	// The config dir is shared through git; the pin is per-machine.
	if err := appendExclude(manifest.Dir(), "host"); err != nil {
		return err
	}
	fmt.Printf("%s this machine is %s in the manifest\n", green("✓"), host.Name())

	if hostFrom == "" {
		fmt.Printf("  %s entries filed under another name stay there — pass --from <old> to move them\n", dim("·"))
		return nil
	}
	moved, err := renameHostInManifest(hostFrom, host.Name())
	if err != nil {
		return err
	}
	fmt.Printf("  %s moved %d entries from %q to %q — run 'repoman sync-manifest' to share it\n",
		green("✓"), moved, hostFrom, host.Name())
	return nil
}
