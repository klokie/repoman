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
	out := Manifest{Defaults: mergeDefaults(base.Defaults, ours.Defaults, theirs.Defaults)}

	bi, oi, ti := indexRepos(base), indexRepos(ours), indexRepos(theirs)

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

	out.Bundles = mergeBundles(base.Bundles, ours.Bundles, theirs.Bundles)

	out.Sort()
	return out
}

func indexRepos(m Manifest) map[string]Repo {
	idx := make(map[string]Repo, len(m.Repos))
	for _, r := range m.Repos {
		idx[r.Name] = r
	}
	return idx
}

// mergeBundles keeps bundles across a semantic merge; before this they were
// simply not carried over, so any conflict resolution silently emptied the
// table. A bundle is named state, so the repo rules apply: a side that dropped
// one it had in the base meant it, anything new on either side survives, and
// ours wins a genuine edit of the same bundle.
func mergeBundles(base, ours, theirs []Bundle) []Bundle {
	index := func(bs []Bundle) map[string]Bundle {
		idx := make(map[string]Bundle, len(bs))
		for _, b := range bs {
			idx[b.Name] = b
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

	var out []Bundle
	for _, name := range sorted {
		_, inBase := bi[name]
		o, inOurs := oi[name]
		t, inTheirs := ti[name]
		if inBase && (!inOurs || !inTheirs) {
			continue // let go of on one side
		}
		merged := o
		if !inOurs {
			merged = t
		}
		out = append(out, merged)
	}
	return out
}

// mergeDefaults resolves [defaults] field by field. Taking one side's table
// wholesale is how a concurrent addition to extra_paths went missing: the
// remote side won the whole table and the local edit vanished with it, leaving
// a directory the user had been told was backed up out of restic entirely.
//
// The lists here are configuration, not membership. extra_paths, backup_skip
// and backup_exclude are the union of what every host wants; a host that never
// knew about an entry is not voting to remove it. Only a side that had the
// entry in the base and dropped it is, which is the one case where removal
// still beats addition.
func mergeDefaults(base, ours, theirs Defaults) Defaults {
	if ours.IsZero() {
		return theirs
	}
	if theirs.IsZero() {
		return ours
	}

	out := ours
	out.Root = pickString(base.Root, ours.Root, theirs.Root)
	out.AssetsRoot = pickString(base.AssetsRoot, ours.AssetsRoot, theirs.AssetsRoot)
	out.ResticRepo = pickString(base.ResticRepo, ours.ResticRepo, theirs.ResticRepo)
	out.ResticPasswordFile = pickString(base.ResticPasswordFile, ours.ResticPasswordFile, theirs.ResticPasswordFile)
	out.ResticMirror = pickString(base.ResticMirror, ours.ResticMirror, theirs.ResticMirror)
	out.ExtraPaths = mergeList(base.ExtraPaths, ours.ExtraPaths, theirs.ExtraPaths)
	out.BackupSkip = mergeList(base.BackupSkip, ours.BackupSkip, theirs.BackupSkip)
	out.BackupExclude = mergeList(base.BackupExclude, ours.BackupExclude, theirs.BackupExclude)
	out.KeepLast = pickInt(base.KeepLast, ours.KeepLast, theirs.KeepLast)
	out.KeepDaily = pickInt(base.KeepDaily, ours.KeepDaily, theirs.KeepDaily)
	out.KeepWeekly = pickInt(base.KeepWeekly, ours.KeepWeekly, theirs.KeepWeekly)
	out.KeepMonthly = pickInt(base.KeepMonthly, ours.KeepMonthly, theirs.KeepMonthly)
	return out
}

// mergeList unions two edits of a configuration list. Entries the base carried
// stay in their original order so the merge produces a one-line diff rather
// than rewriting the list; anything either side added lands after them, sorted,
// so both hosts compute the same result no matter which side they call "ours".
func mergeList(base, ours, theirs []string) []string {
	if len(ours) == 0 && len(theirs) == 0 {
		return nil
	}
	set := func(xs []string) map[string]bool {
		m := make(map[string]bool, len(xs))
		for _, x := range xs {
			m[x] = true
		}
		return m
	}
	o, t := set(ours), set(theirs)

	var out []string
	seen := map[string]bool{}
	for _, x := range base {
		if seen[x] {
			continue
		}
		seen[x] = true // decided here either way, so the union below skips it
		// Dropping an entry that was in the base is a real removal signal.
		if !o[x] || !t[x] {
			continue
		}
		out = append(out, x)
	}

	var added []string
	for _, side := range [][]string{ours, theirs} {
		for _, x := range side {
			if seen[x] {
				continue
			}
			seen[x] = true
			added = append(added, x)
		}
	}
	sort.Strings(added)
	return append(out, added...)
}

// pickString takes whichever side moved away from the base; ours wins when both
// did, matching how a repo's status is resolved.
func pickString(base, ours, theirs string) string {
	if ours == theirs || theirs == base {
		return ours
	}
	if ours == base {
		return theirs
	}
	return ours
}

func pickInt(base, ours, theirs int) int {
	if ours == theirs || theirs == base {
		return ours
	}
	if ours == base {
		return theirs
	}
	return ours
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
