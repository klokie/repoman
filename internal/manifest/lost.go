package manifest

import "fmt"

// LostAdditions reports what a host's own commit added, relative to the base it
// was written against, that did not survive into the merged result.
//
// It exists because the failure it catches is invisible otherwise: when a
// rebase resolves to a tree identical to the upstream tip, the replayed patch
// becomes empty, git drops the commit, nothing errors, and sync-manifest
// happily reports success for an edit that no longer exists anywhere. For
// extra_paths that means a directory the user was told is in restic and isn't.
//
// Only additions are reported. A value the other host deliberately changed is a
// resolved conflict rather than a silent drop, so scalars count as lost only
// when the merge fell all the way back to the base value.
func LostAdditions(base, local, final Manifest) []string {
	var lost []string

	lost = append(lost, lostFromList("defaults.extra_paths", base.Defaults.ExtraPaths, local.Defaults.ExtraPaths, final.Defaults.ExtraPaths)...)
	lost = append(lost, lostFromList("defaults.backup_skip", base.Defaults.BackupSkip, local.Defaults.BackupSkip, final.Defaults.BackupSkip)...)
	lost = append(lost, lostFromList("defaults.backup_exclude", base.Defaults.BackupExclude, local.Defaults.BackupExclude, final.Defaults.BackupExclude)...)

	for _, f := range []struct{ name, base, local, final string }{
		{"defaults.root", base.Defaults.Root, local.Defaults.Root, final.Defaults.Root},
		{"defaults.assets_root", base.Defaults.AssetsRoot, local.Defaults.AssetsRoot, final.Defaults.AssetsRoot},
		{"defaults.restic_repo", base.Defaults.ResticRepo, local.Defaults.ResticRepo, final.Defaults.ResticRepo},
		{"defaults.restic_mirror", base.Defaults.ResticMirror, local.Defaults.ResticMirror, final.Defaults.ResticMirror},
		{"defaults.restic_password_file", base.Defaults.ResticPasswordFile, local.Defaults.ResticPasswordFile, final.Defaults.ResticPasswordFile},
	} {
		if f.local != f.base && f.final == f.base {
			lost = append(lost, fmt.Sprintf("%s = %q", f.name, f.local))
		}
	}

	bi, fi := indexRepos(base), indexRepos(final)
	for _, r := range local.Repos {
		b, inBase := bi[r.Name]
		f, inFinal := fi[r.Name]
		if !inFinal {
			if !inBase {
				lost = append(lost, fmt.Sprintf("repo %q", r.Name))
			}
			continue // a repo the merge pruned was let go of deliberately
		}
		for _, h := range r.Hosts {
			if !b.HasHost(h) && !f.HasHost(h) {
				lost = append(lost, fmt.Sprintf("repo %q on host %q", r.Name, h))
			}
		}
		for h, p := range r.Paths {
			if b.PathOn(h) != p && f.PathOn(h) != p {
				lost = append(lost, fmt.Sprintf("repo %q path on %q = %q", r.Name, h, p))
			}
		}
	}

	inBundles := func(bs []Bundle, name string) bool {
		for _, b := range bs {
			if b.Name == name {
				return true
			}
		}
		return false
	}
	for _, b := range local.Bundles {
		if !inBundles(base.Bundles, b.Name) && !inBundles(final.Bundles, b.Name) {
			lost = append(lost, fmt.Sprintf("bundle %q", b.Name))
		}
	}

	return lost
}

func lostFromList(field string, base, local, final []string) []string {
	in := func(xs []string, want string) bool {
		for _, x := range xs {
			if x == want {
				return true
			}
		}
		return false
	}
	var lost []string
	for _, x := range local {
		if !in(base, x) && !in(final, x) {
			lost = append(lost, fmt.Sprintf("%s: %q", field, x))
		}
	}
	return lost
}
