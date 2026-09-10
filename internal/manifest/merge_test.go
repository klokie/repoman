package manifest

import (
	"strings"
	"testing"
)

func hostsOf(m Manifest, name string) []string {
	i := m.Find(name)
	if i < 0 {
		return nil
	}
	return m.Repos[i].Hosts
}

// The case that broke the real manifest twice: both machines dropped the same
// repo, each rewriting the one hosts line, and git could only see a conflict.
func TestMerge3BothHostsUnassignSameRepo(t *testing.T) {
	base := Manifest{Repos: []Repo{{Name: "rebass", Hosts: []string{"gatekeeper", "oleander"}}}}
	ours := Manifest{Repos: []Repo{{Name: "rebass", Hosts: []string{"oleander"}}}}
	theirs := Manifest{Repos: []Repo{{Name: "rebass", Hosts: []string{"gatekeeper"}}}}

	got := Merge3(base, ours, theirs)
	if len(got.Repos) != 0 {
		t.Fatalf("a repo both hosts let go of should be dropped, got %v", got.Repos)
	}
}

func TestMerge3OneHostUnassignsOtherKeeps(t *testing.T) {
	base := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}}}}
	ours := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}}}}
	theirs := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"oleander"}}}}

	if got := hostsOf(Merge3(base, ours, theirs), "x"); len(got) != 1 || got[0] != "oleander" {
		t.Errorf("removal should win, got %v", got)
	}
}

func TestMerge3ConcurrentAddsBothSurvive(t *testing.T) {
	base := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper"}}}}
	ours := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "metalmark"}}}}
	theirs := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}}}}

	got := hostsOf(Merge3(base, ours, theirs), "x")
	if len(got) != 3 {
		t.Errorf("both additions should survive, got %v", got)
	}
}

func TestMerge3NewReposFromEitherSide(t *testing.T) {
	base := Manifest{}
	ours := Manifest{Repos: []Repo{{Name: "a", Hosts: []string{"gatekeeper"}}}}
	theirs := Manifest{Repos: []Repo{{Name: "b", Hosts: []string{"oleander"}}}}

	got := Merge3(base, ours, theirs)
	if len(got.Repos) != 2 || got.Repos[0].Name != "a" || got.Repos[1].Name != "b" {
		t.Errorf("got %v, want both a and b", got.Repos)
	}
}

func TestMerge3PruneOnOneSideWins(t *testing.T) {
	base := Manifest{Repos: []Repo{{Name: "gone", Hosts: []string{"gatekeeper"}}, {Name: "kept", Hosts: []string{"oleander"}}}}
	ours := Manifest{Repos: []Repo{{Name: "kept", Hosts: []string{"oleander"}}}} // pruned "gone"
	theirs := base

	got := Merge3(base, ours, theirs)
	if got.Find("gone") >= 0 {
		t.Error("a pruned repo must not come back")
	}
	if got.Find("kept") < 0 {
		t.Error("untouched repos must survive")
	}
}

func TestMerge3MergesHostPathsAndStatus(t *testing.T) {
	base := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}, Status: "active"}}}
	ours := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}, Status: "archived"}}}
	theirs := Manifest{Repos: []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}, Status: "active",
		Paths: map[string]string{"oleander": "~/Sites/x"}}}}

	got := Merge3(base, ours, theirs)
	i := got.Find("x")
	if got.Repos[i].Status != "archived" {
		t.Errorf("a deliberate status change should win, got %q", got.Repos[i].Status)
	}
	if got.Repos[i].PathOn("oleander") != "~/Sites/x" {
		t.Errorf("the other side's path override should survive, got %q", got.Repos[i].PathOn("oleander"))
	}
}

func TestEncodeParseRoundTrip(t *testing.T) {
	m := Manifest{Defaults: Defaults{Root: "~/src"}, Repos: []Repo{
		{Name: "x", Hosts: []string{"gatekeeper"}, Paths: map[string]string{"gatekeeper": "~/Sites/x"}}}}
	data, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Repos[0].PathOn("gatekeeper") != "~/Sites/x" {
		t.Errorf("round trip lost the path override: %s", data)
	}
}

func joined(xs []string) string { return strings.Join(xs, ",") }

// The reported loss: two hosts add a different path to extra_paths, and the
// side that syncs second used to have its addition dropped on the floor —
// with the backup silently not covering it.
func TestMerge3UnionsConcurrentExtraPaths(t *testing.T) {
	d := func(paths ...string) Defaults { return Defaults{Root: "~/src", ExtraPaths: paths} }
	base := Manifest{Defaults: d("~/.hermes", "~/.config/repoman")}
	ours := Manifest{Defaults: d("~/.hermes", "~/.config/repoman", "~/from-remote")}
	theirs := Manifest{Defaults: d("~/.hermes", "~/.config/repoman", "~/from-local")}

	got := Merge3(base, ours, theirs).Defaults.ExtraPaths
	if want := "~/.hermes,~/.config/repoman,~/from-local,~/from-remote"; joined(got) != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Both hosts must compute the same list whichever side they call "ours",
// or every later sync churns the line back and forth.
func TestMerge3ExtraPathsMergeIsSymmetric(t *testing.T) {
	d := func(paths ...string) Defaults { return Defaults{ExtraPaths: paths} }
	base := Manifest{Defaults: d("~/a")}
	x := Manifest{Defaults: d("~/a", "~/x")}
	y := Manifest{Defaults: d("~/a", "~/y")}

	if a, b := Merge3(base, x, y), Merge3(base, y, x); joined(a.Defaults.ExtraPaths) != joined(b.Defaults.ExtraPaths) {
		t.Errorf("%v != %v", a.Defaults.ExtraPaths, b.Defaults.ExtraPaths)
	}
}

// Union must not resurrect an entry a host deliberately deleted.
func TestMerge3ExtraPathsRemovalStillWins(t *testing.T) {
	base := Manifest{Defaults: Defaults{ExtraPaths: []string{"~/a", "~/gone"}}}
	ours := Manifest{Defaults: Defaults{ExtraPaths: []string{"~/a", "~/gone"}}}
	theirs := Manifest{Defaults: Defaults{ExtraPaths: []string{"~/a"}}}

	if got := Merge3(base, ours, theirs).Defaults.ExtraPaths; joined(got) != "~/a" {
		t.Errorf("got %v, want ~/a", got)
	}
}

func TestMerge3DefaultsScalarChangeOnEitherSide(t *testing.T) {
	base := Manifest{Defaults: Defaults{Root: "~/src", ResticRepo: "old", KeepDaily: 7}}
	ours := Manifest{Defaults: Defaults{Root: "~/src", ResticRepo: "old", KeepDaily: 14}}
	theirs := Manifest{Defaults: Defaults{Root: "~/src", ResticRepo: "new", KeepDaily: 7}}

	got := Merge3(base, ours, theirs).Defaults
	if got.ResticRepo != "new" {
		t.Errorf("restic_repo = %q, want the changed side", got.ResticRepo)
	}
	if got.KeepDaily != 14 {
		t.Errorf("keep_daily = %d, want 14", got.KeepDaily)
	}
}

// Bundles used to be dropped entirely by any semantic merge.
func TestMerge3KeepsBundles(t *testing.T) {
	base := Manifest{Bundles: []Bundle{{Name: "acme"}}}
	ours := Manifest{Bundles: []Bundle{{Name: "acme"}, {Name: "from-remote"}}}
	theirs := Manifest{Bundles: []Bundle{{Name: "acme"}, {Name: "from-local"}}}

	got := Merge3(base, ours, theirs).Bundles
	if len(got) != 3 {
		t.Fatalf("got %v, want all three bundles", got)
	}
}

func TestLostAdditionsFindsDroppedExtraPath(t *testing.T) {
	base := Manifest{Defaults: Defaults{ExtraPaths: []string{"~/a"}}}
	local := Manifest{Defaults: Defaults{ExtraPaths: []string{"~/a", "~/mine"}}}
	final := Manifest{Defaults: Defaults{ExtraPaths: []string{"~/a", "~/theirs"}}}

	lost := LostAdditions(base, local, final)
	if len(lost) != 1 || !strings.Contains(lost[0], "~/mine") {
		t.Errorf("got %v, want the dropped path reported", lost)
	}
}

func TestLostAdditionsQuietOnAGoodMerge(t *testing.T) {
	base := Manifest{
		Defaults: Defaults{ExtraPaths: []string{"~/a"}},
		Repos:    []Repo{{Name: "x", Hosts: []string{"gatekeeper"}}},
	}
	local := Manifest{
		Defaults: Defaults{ExtraPaths: []string{"~/a", "~/mine"}},
		Repos:    []Repo{{Name: "x", Hosts: []string{"gatekeeper", "metalmark"}}},
	}
	final := Merge3(base, Manifest{
		Defaults: Defaults{ExtraPaths: []string{"~/a", "~/theirs"}},
		Repos:    []Repo{{Name: "x", Hosts: []string{"gatekeeper", "oleander"}}},
	}, local)

	if lost := LostAdditions(base, local, final); len(lost) != 0 {
		t.Errorf("nothing was lost, got %v", lost)
	}
}

// A repo the other host pruned, or a value it deliberately changed, is a
// resolved conflict rather than a silent drop.
func TestLostAdditionsIgnoresDeliberateResolutions(t *testing.T) {
	base := Manifest{
		Defaults: Defaults{ResticRepo: "old"},
		Repos:    []Repo{{Name: "x", Hosts: []string{"gatekeeper"}}},
	}
	local := Manifest{
		Defaults: Defaults{ResticRepo: "mine"},
		Repos:    []Repo{{Name: "x", Hosts: []string{"gatekeeper"}}},
	}
	final := Manifest{Defaults: Defaults{ResticRepo: "theirs"}} // repo pruned there

	if lost := LostAdditions(base, local, final); len(lost) != 0 {
		t.Errorf("got %v, want nothing reported", lost)
	}
}
