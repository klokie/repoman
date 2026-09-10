package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klokie/repoman/internal/manifest"
)

// seedManifestRemote returns a bare remote holding m, plus one clone of it.
func seedManifestRemote(t *testing.T, m manifest.Manifest) (remote, clone string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "manifest.git")
	git(t, root, "init", "-q", "--bare", "-b", "main", remote)

	clone = filepath.Join(root, "hostA")
	git(t, root, "clone", "-q", remote, clone)
	writeManifest(t, clone, m)
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-qm", "seed")
	git(t, clone, "push", "-q", "-u", "origin", "HEAD:main")
	git(t, clone, "branch", "-q", "--set-upstream-to=origin/main")
	return remote, clone
}

func writeManifest(t *testing.T, dir string, m manifest.Manifest) {
	t.Helper()
	t.Setenv("REPOMAN_CONFIG", dir)
	if err := manifest.Save(m); err != nil {
		t.Fatal(err)
	}
}

func readManifest(t *testing.T, dir string) manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func syncIn(t *testing.T, dir string) error {
	t.Helper()
	t.Setenv("REPOMAN_CONFIG", dir)
	return runSyncManifest(nil, nil)
}

// The issue as reported: host B syncs an extra_paths addition first, then host
// A syncs its own. Host A's path used to vanish while the command exited 0.
func TestSyncManifestKeepsConcurrentExtraPathAdditions(t *testing.T) {
	setGitIdentity(t)
	base := manifest.Manifest{
		Defaults: manifest.Defaults{Root: "~/src", ExtraPaths: []string{"~/.hermes"}},
		Repos:    []manifest.Repo{{Name: "x", Remote: "https://example.com/x.git", Hosts: []string{"hosta"}}},
	}
	remote, hostA := seedManifestRemote(t, base)

	hostB := filepath.Join(filepath.Dir(hostA), "hostB")
	git(t, filepath.Dir(hostA), "clone", "-q", remote, hostB)

	b := base
	b.Defaults.ExtraPaths = []string{"~/.hermes", "~/from-b"}
	writeManifest(t, hostB, b)
	if err := syncIn(t, hostB); err != nil {
		t.Fatalf("host B sync: %v", err)
	}

	a := base
	a.Defaults.ExtraPaths = []string{"~/.hermes", "~/from-a"}
	writeManifest(t, hostA, a)
	if err := syncIn(t, hostA); err != nil {
		t.Fatalf("host A sync: %v", err)
	}

	got := strings.Join(readManifest(t, hostA).Defaults.ExtraPaths, ",")
	if want := "~/.hermes,~/from-a,~/from-b"; got != want {
		t.Errorf("extra_paths = %q, want %q", got, want)
	}

	// And the union has to reach the remote, not just this working copy.
	git(t, hostB, "pull", "-q", "--rebase")
	if got := strings.Join(readManifest(t, hostB).Defaults.ExtraPaths, ","); !strings.Contains(got, "~/from-a") {
		t.Errorf("host B did not receive the addition: %q", got)
	}
}

func setGitIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
}

// Whatever else goes wrong in a merge, a sync that threw this host's edit away
// must not report success.
func TestSyncManifestFailsWhenTheMergeDropsTheLocalEdit(t *testing.T) {
	setGitIdentity(t)
	base := manifest.Manifest{
		Defaults: manifest.Defaults{Root: "~/src"},
		Repos:    []manifest.Repo{{Name: "x", Remote: "https://example.com/x.git", Hosts: []string{"hosta"}}},
	}
	_, dir := seedManifestRemote(t, base)

	// Commit an addition, then rewind the working tree to the base so the sync
	// finds a tree with the edit missing — the shape a dropped rebase leaves.
	local := base
	local.Defaults.ExtraPaths = []string{"~/mine"}
	writeManifest(t, dir, local)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "local edit")
	sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	git(t, dir, "reset", "-q", "--hard", "HEAD~1")

	lost, err := lostLocalEdits(dir, sha)
	if err != nil {
		t.Fatal(err)
	}
	if len(lost) != 1 || !strings.Contains(lost[0], "~/mine") {
		t.Fatalf("got %v, want the dropped extra_paths entry", lost)
	}
}
