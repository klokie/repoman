package manifest

import "testing"

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
