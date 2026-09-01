// Package gitx wraps the git and shell calls repoman shells out to.
package gitx

import (
	"fmt"
	"os/exec"
	"strings"
)

// Output runs git in dir and returns trimmed stdout.
func Output(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	var stderr strings.Builder
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// Run runs git in dir, discarding stdout but surfacing stderr in the error.
func Run(dir string, args ...string) error {
	_, err := Output(dir, args...)
	return err
}

func IsRepo(dir string) bool {
	out, err := Output(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

func RemoteURL(dir string) string {
	out, err := Output(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return out
}

func CurrentBranch(dir string) string {
	out, _ := Output(dir, "rev-parse", "--abbrev-ref", "HEAD")
	return out
}

func IsDirty(dir string) bool {
	out, err := Output(dir, "status", "--porcelain")
	return err == nil && out != ""
}

// HasUpstream reports whether the current branch tracks a remote branch.
func HasUpstream(dir string) bool {
	_, err := Output(dir, "rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

// NameFromURL derives a repo directory name from a clone URL:
// git@github.com:klokie/repoman.git -> repoman
func NameFromURL(url string) string {
	u := strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(url, "/"), ".git"), "/")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	return u
}
