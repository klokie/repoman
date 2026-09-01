package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNamePrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REPOMAN_CONFIG", dir)

	// A drifted OS hostname is what the pin file exists to override.
	if err := os.WriteFile(filepath.Join(dir, "host"), []byte("Oleander\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Name(); got != "oleander" {
		t.Errorf("pin file should win over the OS hostname, got %q", got)
	}

	t.Setenv("REPOMAN_HOST", "Metalmark.local")
	if got := Name(); got != "metalmark" {
		t.Errorf("env should win over the pin file, got %q", got)
	}
}

func TestSetNormalizes(t *testing.T) {
	t.Setenv("REPOMAN_CONFIG", t.TempDir())
	t.Setenv("REPOMAN_HOST", "")
	if err := Set(" Gatekeeper.Local \n"); err != nil {
		t.Fatal(err)
	}
	if got := Name(); got != "gatekeeper" {
		t.Errorf("got %q, want gatekeeper", got)
	}
}
