package manifest

import (
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Merge3 resolves two manifests that diverged from a common base.
//
// Git cannot merge this file usefully: two machines unassigning the same repo
// both rewrite one `hosts = [...]` line and conflict every time, even though
// the intent — "neither of us wants it" — is unambiguous. Merging on meaning
// instead of text: host membership is a set, so an add on either side wins over
// the base, and a removal on either side wins over an add.
func Merge3(base, ours, theirs Manifest) Manifest {
	out := Manifest{Defaults: ours.Defaults}
	if out.Defaults == (Defaults{}) {
		out.Defaults = theirs.Defaults
	}

	index := func(m Manifest) map[string]Repo {
		idx := make(map[string]Repo, len(m.Repos))
		for _, r := range m.Repos {
			idx[r.Name] = r
		}
		return idx
	}
	bi, oi, ti := index(base), index(ours), index(theirs)

	names := map[string]bool{}
	for n := range oi {
		names[n] = true
	}
	for n := range ti {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		b, inBase := bi[name]
		o, inOurs := oi[name]
		t, inTheirs := ti[name]

		// A repo dropped on one side after being in the base was pruned there;
		// honor the deletion rather than resurrecting it.
		if inBase && (!inOurs || !inTheirs) {
			continue
		}

		merged := o
		if !inOurs {
			merged = t
		}
		merged.Hosts = mergeHosts(b.Hosts, o.Hosts, t.Hosts, inBase)
		merged.Paths = mergePaths(b.Paths, o.Paths, t.Paths)
		if merged.Remote == "" {
			merged.Remote = firstNonEmpty(o.Remote, t.Remote)
		}
		// A status change on either side is deliberate; base means "unchanged".
		if inBase {
			if o.Status != b.Status {
				merged.Status = o.Status
			} else if t.Status != b.Status {
				merged.Status = t.Status
			}
		}
		if len(merged.Hosts) == 0 {
			continue // every host let go of it
		}
		out.Repos = append(out.Repos, merged)
	}

	out.Sort()
	return out
}

func mergeHosts(base, ours, theirs []string, inBase bool) []string {
	set := func(xs []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			m[strings.ToLower(x)] = true
		}
		return m
	}
	b, o, t := set(base), set(ours), set(theirs)

	result := map[string]bool{}
	for h := range o {
		result[h] = true
	}
	for h := range t {
		result[h] = true
	}
	if inBase {
		// Removal beats addition: if a side dropped a host it had in the base,
		// that host meant to let go.
		for h := range b {
			if !o[h] || !t[h] {
				delete(result, h)
			}
		}
	}

	hosts := make([]string, 0, len(result))
	for h := range result {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	return hosts
}

// mergePaths unions per-host overrides; each host only ever writes its own key,
// so a plain union is right, with ours winning a genuine collision.
func mergePaths(base, ours, theirs map[string]string) map[string]string {
	if len(ours) == 0 && len(theirs) == 0 {
		return nil
	}
	out := map[string]string{}
	for h, p := range theirs {
		out[h] = p
	}
	for h, p := range ours {
		out[h] = p
	}
	// A host that deleted its own override on one side meant it.
	for h := range base {
		_, inOurs := ours[h]
		_, inTheirs := theirs[h]
		if !inOurs || !inTheirs {
			delete(out, h)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Parse reads a manifest from raw TOML, for merging git index stages.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	err := toml.Unmarshal(data, &m)
	return m, err
}

// Encode renders a manifest back to TOML.
func Encode(m Manifest) ([]byte, error) {
	m.Sort()
	var sb strings.Builder
	if err := toml.NewEncoder(&sb).Encode(m); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}
