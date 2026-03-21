package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Manifest struct {
	Defaults Defaults `toml:"defaults"`
	Repos    []Repo   `toml:"repos"`
}

type Defaults struct {
	Root       string `toml:"root"`
	ResticRepo string `toml:"restic_repo,omitempty"`
}

type Repo struct {
	Name   string   `toml:"name"`
	Remote string   `toml:"remote"`
	Path   string   `toml:"path,omitempty"`
	Hosts  []string `toml:"hosts"`
	Tags   []string `toml:"tags,omitempty"`
	Status string   `toml:"status,omitempty"`
}

func (r Repo) ExpandedPath() string {
	p := r.Path
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[2:])
	}
	return p
}

func (m Manifest) ReposForHost(hostname string) []Repo {
	var result []Repo
	for _, r := range m.Repos {
		for _, h := range r.Hosts {
			if strings.EqualFold(h, hostname) {
				result = append(result, r)
				break
			}
		}
	}
	return result
}

func Path() string {
	configDir := os.Getenv("REPOMAN_CONFIG")
	if configDir != "" {
		return filepath.Join(configDir, "manifest.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repoman", "manifest.toml")
}

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

func Save(m Manifest) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(m)
}
