package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Manifest struct {
	Defaults Defaults `toml:"defaults"`
	Repos    []Repo   `toml:"repos"`
	Bundles  []Bundle `toml:"bundles,omitempty"`
}

// A Bundle groups repos with the non-git asset directory that belongs to the
// same project, so a client engagement can be shelved and brought back as one
// unit instead of six separate operations.
type Bundle struct {
	Name   string   `toml:"name"`
	Repos  []string `toml:"repos,omitempty"`
	Assets string   `toml:"assets,omitempty"` // relative to assets_root
	Tags   []string `toml:"tags,omitempty"`
	Status string   `toml:"status,omitempty"`
}

type Defaults struct {
	Root               string `toml:"root"`
	AssetsRoot         string `toml:"assets_root,omitempty"`
	ResticRepo         string `toml:"restic_repo,omitempty"`
	ResticPasswordFile string `toml:"restic_password_file,omitempty"`
	// ResticMirror is a second repo the snapshots are copied to — a drive that
	// something else already carries offsite.
	ResticMirror string `toml:"restic_mirror,omitempty"`
	// ExtraPaths are backed up as-is: state that belongs to no repo, like
	// ~/.hermes. Paths absent on a given host are skipped silently, so one
	// list can serve every machine.
	ExtraPaths []string `toml:"extra_paths,omitempty"`
	// BackupSkip lists repo-name globs to leave out of `repoman backup`.
	// Employer credentials do not belong on personal storage, however good the
	// intent, so the exclusion lives in the shared manifest rather than in a
	// flag someone has to remember.
	BackupSkip []string `toml:"backup_skip,omitempty"`
	// BackupExclude drops individual paths, for secrets that sit inside a repo
	// that is otherwise worth keeping. Globs match the ~-contracted path.
	BackupExclude []string `toml:"backup_exclude,omitempty"`
	// Retention ladder for `repoman forget`. Zero means "use the built-in
	// default"; pruning stays a deliberate, separate step either way.
	KeepLast    int `toml:"keep_last,omitempty"`
	KeepDaily   int `toml:"keep_daily,omitempty"`
	KeepWeekly  int `toml:"keep_weekly,omitempty"`
	KeepMonthly int `toml:"keep_monthly,omitempty"`
}

type Repo struct {
	Name   string   `toml:"name"`
	Remote string   `toml:"remote"`
	Path   string   `toml:"path,omitempty"`
	Hosts  []string `toml:"hosts"`
	Tags   []string `toml:"tags,omitempty"`
	Status string   `toml:"status,omitempty"`
	// Paths overrides the location per host, for the repos that do not sit at
	// <root>/<name> everywhere — acme-api is ~/src/acme-api on gatekeeper
	// but ~/Sites/acme-api on oleander. Path (no host) still works as a
	// manifest-wide override.
	Paths map[string]string `toml:"paths,omitempty"`
}

// Expand resolves a leading ~/ against the user's home directory.
func Expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// Contract is the inverse of Expand: absolute paths under $HOME become ~/…
func Contract(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return p
}

func (r Repo) HasHost(hostname string) bool {
	for _, h := range r.Hosts {
		if strings.EqualFold(h, hostname) {
			return true
		}
	}
	return false
}

func (r Repo) IsArchived() bool { return r.Status == "archived" }

// ExpandedPath returns the repo's own path only; prefer Manifest.PathFor, which
// falls back to the default root when a repo has no explicit path.
func (r Repo) ExpandedPath() string {
	if r.Path == "" {
		return ""
	}
	return Expand(r.Path)
}

// RootDir is the default clone root, expanded. Defaults to ~/src.
// IsZero reports whether no defaults were set at all.
func (d Defaults) IsZero() bool {
	return d.Root == "" && d.AssetsRoot == "" && d.ResticRepo == "" && d.ResticMirror == "" &&
		d.ResticPasswordFile == "" && len(d.ExtraPaths) == 0 && len(d.BackupSkip) == 0 && len(d.BackupExclude) == 0 &&
		d.KeepLast == 0 && d.KeepDaily == 0 && d.KeepWeekly == 0 && d.KeepMonthly == 0
}

func (m Manifest) RootDir() string {
	root := m.Defaults.Root
	if root == "" {
		root = "~/src"
	}
	return Expand(root)
}

// PathFor resolves where a repo lives on a given host: the host's own override
// first, then a manifest-wide path, then <default root>/<name>.
func (m Manifest) PathFor(r Repo, hostname string) string {
	if p := r.PathOn(hostname); p != "" {
		return Expand(p)
	}
	if p := r.ExpandedPath(); p != "" {
		return p
	}
	return filepath.Join(m.RootDir(), r.Name)
}

// PathOn returns the host-specific path override, if any.
func (r Repo) PathOn(hostname string) string {
	for h, p := range r.Paths {
		if strings.EqualFold(h, hostname) {
			return p
		}
	}
	return ""
}

// SetPathOn records where this repo lives on a host.
func (r *Repo) SetPathOn(hostname, path string) {
	if r.Paths == nil {
		r.Paths = map[string]string{}
	}
	for h := range r.Paths {
		if strings.EqualFold(h, hostname) {
			r.Paths[h] = path
			return
		}
	}
	r.Paths[strings.ToLower(hostname)] = path
}

func (m Manifest) ReposForHost(hostname string) []Repo {
	var result []Repo
	for _, r := range m.Repos {
		if r.HasHost(hostname) {
			result = append(result, r)
		}
	}
	return result
}

// Hosts lists every host mentioned in the manifest, sorted.
func (m Manifest) Hosts() []string {
	seen := map[string]bool{}
	var hosts []string
	for _, r := range m.Repos {
		for _, h := range r.Hosts {
			k := strings.ToLower(h)
			if !seen[k] {
				seen[k] = true
				hosts = append(hosts, k)
			}
		}
	}
	sort.Strings(hosts)
	return hosts
}

// FindBundle returns the index of a bundle by name, or -1.
func (m Manifest) FindBundle(name string) int {
	for i, b := range m.Bundles {
		if b.Name == name {
			return i
		}
	}
	return -1
}

// AssetsDir resolves a bundle's asset directory, or "" when it has none.
func (m Manifest) AssetsDir(b Bundle) string {
	if b.Assets == "" {
		return ""
	}
	root := m.Defaults.AssetsRoot
	if root == "" {
		root = "~/projects"
	}
	return filepath.Join(Expand(root), b.Assets)
}

// Find returns the index of a repo by name, or -1.
func (m Manifest) Find(name string) int {
	for i, r := range m.Repos {
		if r.Name == name {
			return i
		}
	}
	return -1
}

// Upsert adds a repo, or merges it into an existing entry of the same name.
// Reports whether the repo was new, whether an existing entry gained a host,
// and whether anything at all changed (a path or backfilled remote counts, and
// is why re-running init on a host can still be worth saving).
func (m *Manifest) Upsert(r Repo) (added, hostAdded, changed bool) {
	i := m.Find(r.Name)
	if i < 0 {
		m.Repos = append(m.Repos, r)
		return true, false, true
	}
	existing := &m.Repos[i]
	for _, h := range r.Hosts {
		if !existing.HasHost(h) {
			existing.Hosts = append(existing.Hosts, h)
			hostAdded = true
		}
	}
	if existing.Remote == "" && r.Remote != "" {
		existing.Remote = r.Remote
		changed = true
	}
	for h, p := range r.Paths {
		if existing.PathOn(h) != p {
			existing.SetPathOn(h, p)
			changed = true
		}
	}
	return false, hostAdded, hostAdded || changed
}

// RemoveHost drops a host from a repo. Returns false if the repo isn't found.
func (m *Manifest) RemoveHost(name, hostname string) bool {
	i := m.Find(name)
	if i < 0 {
		return false
	}
	var kept []string
	for _, h := range m.Repos[i].Hosts {
		if !strings.EqualFold(h, hostname) {
			kept = append(kept, h)
		}
	}
	m.Repos[i].Hosts = kept
	return true
}

func (m *Manifest) Sort() {
	sort.Slice(m.Repos, func(i, j int) bool { return m.Repos[i].Name < m.Repos[j].Name })
	for i := range m.Repos {
		sort.Strings(m.Repos[i].Hosts)
	}
}

// Dir is the config directory holding manifest.toml (REPOMAN_CONFIG overrides).
func Dir() string {
	if d := os.Getenv("REPOMAN_CONFIG"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repoman")
}

func Path() string { return filepath.Join(Dir(), "manifest.toml") }

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func Load() (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(Path())
	if err != nil {
		return m, fmt.Errorf("reading %s: %w", Path(), err)
	}
	if err := toml.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parsing %s: %w", Path(), err)
	}
	return m, nil
}

// Save writes the manifest atomically, sorted, so that two hosts editing it
// produce diffs git can merge instead of whole-file churn.
func Save(m Manifest) error {
	m.Sort()
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".manifest-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := toml.NewEncoder(tmp).Encode(m); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
