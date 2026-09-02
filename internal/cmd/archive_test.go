package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/klokie/repoman/internal/manifest"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// newClonedRepo builds a repo with a bare remote and one pushed commit.
func newClonedRepo(t *testing.T) (path, remote string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "origin.git")
	git(t, root, "init", "-q", "--bare", remote)
	path = filepath.Join(root, "work")
	git(t, root, "clone", "-q", remote, path)
	if err := os.WriteFile(filepath.Join(path, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, path, "add", "-A")
	git(t, path, "commit", "-qm", "init")
	git(t, path, "push", "-q", "-u", "origin", "HEAD")
	return path, remote
}

// Deleting a clone is only safe when the remote genuinely has everything;
// these are the cases where it does not.
func TestSafetyCheckRefusesUnsafeArchives(t *testing.T) {
	m := manifest.Manifest{}

	t.Run("clean and pushed is safe", func(t *testing.T) {
		path, remote := newClonedRepo(t)
		if why := safetyCheck(m, manifest.Repo{Name: "x", Remote: remote}, path); why != "" {
			t.Errorf("expected safe, got %q", why)
		}
	})

	t.Run("uncommitted changes", func(t *testing.T) {
		path, remote := newClonedRepo(t)
		os.WriteFile(filepath.Join(path, "file.txt"), []byte("edited\n"), 0o644)
		if why := safetyCheck(m, manifest.Repo{Name: "x", Remote: remote}, path); why != "uncommitted changes" {
			t.Errorf("got %q", why)
		}
	})

	t.Run("unpushed commits", func(t *testing.T) {
		path, remote := newClonedRepo(t)
		os.WriteFile(filepath.Join(path, "file.txt"), []byte("more\n"), 0o644)
		git(t, path, "commit", "-aqm", "local only")
		if why := safetyCheck(m, manifest.Repo{Name: "x", Remote: remote}, path); why != "1 unpushed commit(s)" {
			t.Errorf("got %q", why)
		}
	})

	t.Run("no remote is never safe", func(t *testing.T) {
		path, _ := newClonedRepo(t)
		if why := safetyCheck(m, manifest.Repo{Name: "x"}, path); why == "" {
			t.Error("a repo with no remote must never be archivable without --force")
		}
	})

	t.Run("stashes would be lost", func(t *testing.T) {
		path, remote := newClonedRepo(t)
		os.WriteFile(filepath.Join(path, "file.txt"), []byte("stashed\n"), 0o644)
		git(t, path, "stash", "push", "-q", "-m", "wip")
		if why := safetyCheck(m, manifest.Repo{Name: "x", Remote: remote}, path); why == "" {
			t.Error("a stash is local-only state; archiving must not silently drop it")
		}
	})

	t.Run("already deleted is a no-op", func(t *testing.T) {
		if why := safetyCheck(m, manifest.Repo{Name: "x", Remote: "r"}, "/nonexistent/path"); why != "" {
			t.Errorf("got %q", why)
		}
	})
}
