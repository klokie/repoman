package host

import (
	"os"
	"path/filepath"
	"strings"
)

// Name is this machine's identity in the manifest: short and lowercased.
//
// It is deliberately not just os.Hostname(): macOS renames a Mac to
// "oleander-5" after a Bonjour name collision, which would silently split one
// host's repos across two identities. Resolution order:
//
//	$REPOMAN_HOST  →  <config dir>/host  →  os.Hostname()
func Name() string {
	if h := os.Getenv("REPOMAN_HOST"); h != "" {
		return normalize(h)
	}
	if h, err := os.ReadFile(HostFile()); err == nil {
		if n := normalize(string(h)); n != "" {
			return n
		}
	}
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return normalize(h)
}

// Detected is the raw hostname the OS reports, for showing the difference.
func Detected() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return normalize(h)
}

// HostFile is the local (never shared) file pinning this machine's name.
func HostFile() string { return filepath.Join(configDir(), "host") }

// Set pins this machine's name and keeps it out of the shared manifest repo.
func Set(name string) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(HostFile(), []byte(normalize(name)+"\n"), 0o644)
}

// configDir mirrors manifest.Dir; duplicated to keep this package dependency-free.
func configDir() string {
	if d := os.Getenv("REPOMAN_CONFIG"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repoman")
}

func normalize(h string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(h, ".")[0]))
}
