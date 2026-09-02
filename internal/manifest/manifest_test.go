package manifest

import (
	"path/filepath"
	"testing"
)

func TestUpsertAddsHostInsteadOfDuplicating(t *testing.T) {
	m := Manifest{Repos: []Repo{{Name: "repoman", Remote: "git@github.com:klokie/repoman.git", Hosts: []string{"gatekeeper"}}}}

	added, hostAdded, _ := m.Upsert(Repo{Name: "repoman", Hosts: []string{"oleander"}})
	if added || !hostAdded {
		t.Fatalf("second host: got added=%v hostAdded=%v, want false/true", added, hostAdded)
	}
	if len(m.Repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(m.Repos))
	}
	if !m.Repos[0].HasHost("Oleander") {
		t.Error("host match should be case-insensitive")
	}

	if _, hostAdded, changed := m.Upsert(Repo{Name: "repoman", Hosts: []string{"oleander"}}); hostAdded || changed {
		t.Error("re-running init on the same host should be a no-op")
	}

	if added, _, _ := m.Upsert(Repo{Name: "hermes", Hosts: []string{"metalmark"}}); !added {
		t.Error("a new repo should be added")
	}
}

func TestUpsertBackfillsMissingRemote(t *testing.T) {
	m := Manifest{Repos: []Repo{{Name: "orphan", Hosts: []string{"gatekeeper"}}}}
	m.Upsert(Repo{Name: "orphan", Remote: "git@github.com:klokie/orphan.git", Hosts: []string{"metalmark"}})
	if m.Repos[0].Remote == "" {
		t.Error("a host that knows the remote should fill in the blank")
	}
}

func TestPathForFallsBackToRoot(t *testing.T) {
	m := Manifest{Defaults: Defaults{Root: "~/code"}}
	got := m.PathFor(Repo{Name: "repoman"}, "gatekeeper")
	if want := filepath.Join(Expand("~/code"), "repoman"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	explicit := m.PathFor(Repo{Name: "repoman", Path: "~/src/_archived/repoman"}, "gatekeeper")
	if want := Expand("~/src/_archived/repoman"); explicit != want {
		t.Errorf("explicit path: got %q, want %q", explicit, want)
	}
}

// The same repo lives in a different directory on each machine; one global
// path cannot express that, and getting it wrong reports phantom "missing".
func TestPathForPrefersHostOverride(t *testing.T) {
	m := Manifest{Defaults: Defaults{Root: "~/src"}}
	r := Repo{Name: "acme-api", Paths: map[string]string{"oleander": "~/Sites/acme-api"}}

	if got, want := m.PathFor(r, "Oleander"), Expand("~/Sites/acme-api"); got != want {
		t.Errorf("oleander: got %q, want %q", got, want)
	}
	if got, want := m.PathFor(r, "gatekeeper"), Expand("~/src/acme-api"); got != want {
		t.Errorf("gatekeeper should fall back to the root: got %q, want %q", got, want)
	}
}

func TestUpsertMergesHostPaths(t *testing.T) {
	m := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper"}}}}
	incoming := Repo{Name: "x", Hosts: []string{"oleander"}}
	incoming.SetPathOn("oleander", "~/Sites/x")
	m.Upsert(incoming)

	if got := m.Repos[0].PathOn("oleander"); got != "~/Sites/x" {
		t.Errorf("got %q, want ~/Sites/x", got)
	}
	if got := m.Repos[0].PathOn("gatekeeper"); got != "" {
		t.Errorf("other hosts should be untouched, got %q", got)
	}
}

func TestContractRoundTrips(t *testing.T) {
	if got := Contract(Expand("~/src/repoman")); got != "~/src/repoman" {
		t.Errorf("got %q, want ~/src/repoman", got)
	}
	if got := Contract("/Volumes/Emperor/x"); got != "/Volumes/Emperor/x" {
		t.Errorf("paths outside home must stay absolute, got %q", got)
	}
}

func TestHostsAndReposForHost(t *testing.T) {
	m := Manifest{Repos: []Repo{
		{Name: "a", Hosts: []string{"Gatekeeper"}},
		{Name: "b", Hosts: []string{"metalmark", "gatekeeper"}},
	}}
	hosts := m.Hosts()
	if len(hosts) != 2 || hosts[0] != "gatekeeper" || hosts[1] != "metalmark" {
		t.Errorf("got %v, want [gatekeeper metalmark]", hosts)
	}
	if len(m.ReposForHost("gatekeeper")) != 2 {
		t.Error("gatekeeper should carry both repos")
	}
	if len(m.ReposForHost("oleander")) != 0 {
		t.Error("unknown host should carry nothing")
	}
}

func TestSaveLoadRoundTripSorts(t *testing.T) {
	t.Setenv("REPOMAN_CONFIG", t.TempDir())
	m := Manifest{
		Defaults: Defaults{Root: "~/src", AssetsRoot: "~/projects"},
		Repos: []Repo{
			{Name: "zebra", Hosts: []string{"metalmark"}, Status: "active", Paths: map[string]string{"metalmark": "~/work/zebra"}},
			{Name: "alpha", Hosts: []string{"oleander", "gatekeeper"}, Status: "archived"},
		},
	}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Repos[0].Name != "alpha" || got.Repos[1].Name != "zebra" {
		t.Errorf("repos should be sorted by name, got %v", got.Repos)
	}
	if got.Repos[0].Hosts[0] != "gatekeeper" {
		t.Errorf("hosts should be sorted, got %v", got.Repos[0].Hosts)
	}
	if got.Defaults.AssetsRoot != "~/projects" {
		t.Error("defaults did not round-trip")
	}
	if p := got.Repos[1].PathOn("metalmark"); p != "~/work/zebra" {
		t.Errorf("per-host paths did not round-trip through TOML, got %q", p)
	}
}
