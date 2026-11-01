package cmd

import (
	"reflect"
	"testing"

	"github.com/klokie/repoman/internal/manifest"
)

func TestSnapshotHostsToCheckIgnoresRetiredHistoricalHosts(t *testing.T) {
	m := manifest.Manifest{Repos: []manifest.Repo{
		{Name: "active", Hosts: []string{"metalmark", "gatekeeper"}},
		{Name: "retired-archive", Hosts: []string{"oleander"}, Status: "archived"},
	}}
	got := snapshotHostsToCheck(m)
	want := []string{"gatekeeper", "metalmark"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("freshness should follow current manifest hosts, got %v want %v", got, want)
	}
}
