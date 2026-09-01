# repoman

Multi-machine git repository manager. One command to manage hundreds of repos across multiple machines with per-host manifests and restic backup.

## Why

Cloud sync tools (Google Drive, iCloud, Dropbox, Syncthing) and git repos don't mix. Continuous background sync creates conflict copies (`file (1).ext`), corrupts `.git` internals, and fights git for file locks. Repoman replaces cloud sync for source code with explicit, git-native workflows.

## What it does

- **Per-machine manifests** — each machine gets a TOML config listing which repos it needs
- **Parallel status** — branch, dirty state, and unpushed commits across all repos in seconds
- **Clone + register** — `repoman clone <url>` clones and adds to the manifest in one step
- **Archive / restore** — shelve inactive projects, bring them back months later
- **Backup non-git state** — `.env`, local config, untracked assets backed up to S3-compatible storage via restic

## Install

```bash
go install github.com/klokie/repoman@latest
```

Or from a local clone:

```bash
git clone https://github.com/klokie/repoman.git
cd repoman
go install ./cmd/repoman
```

`go install` places the binary in `$GOBIN` (or `$GOPATH/bin`, defaulting to `~/go/bin`). Make sure that directory is on your PATH:

```bash
# add to ~/.bashrc, ~/.zshrc, or equivalent
export PATH="$PATH:$(go env GOPATH)/bin"
```

After that, `repoman` is available everywhere.

## Quick start

On the first machine:

```bash
repoman init                                                  # scan ~/src, record repos under this host
repoman sync-manifest --init git@github.com:you/manifest.git  # publish the manifest
repoman status
```

On every other machine:

```bash
repoman sync-manifest --init git@github.com:you/manifest.git  # get the shared manifest
repoman init                                                  # add whatever this host already has
repoman assign werlabs-js hermes                              # claim repos you want here
repoman clone                                                 # clone everything assigned but missing
repoman sync-manifest                                         # share the change
```

Daily:

```bash
repoman pull      # fast-forward every active repo, in parallel
repoman status    # what is dirty, unpushed, or missing
```

## Commands

| Command                    | Description                                                                  |
| -------------------------- | ---------------------------------------------------------------------------- |
| `repoman init`             | Scan `~/src` (+ `~/Sites`) and record repos under this host. Safe to re-run. |
| `repoman status`           | Branch, dirty/clean, unpushed, missing — in parallel. `--problems`, `--host` |
| `repoman hosts`            | Every host in the manifest and how many repos it carries                     |
| `repoman assign <repo>...` | Claim repos for this host (`--tag`, `--host`); `unassign` is the inverse     |
| `repoman clone`            | Clone every active repo assigned here but not present                        |
| `repoman clone <url>`      | Clone one repo into the root and register it                                 |
| `repoman pull`             | Fast-forward all active repos in parallel (`--rebase`, `--autostash`, `-j`)  |
| `repoman sync-manifest`    | Pull/commit/push the manifest; `--init <remote>` attaches a machine to it    |
| `repoman archive <name>`   | Backup local state, remove clone, mark archived _(planned)_                  |
| `repoman restore <name>`   | Clone from remote + restore .env from restic _(planned)_                     |
| `repoman backup`           | Restic snapshot of non-git files _(planned)_                                 |

`init`, `assign` and `clone` never overwrite an entry that another host wrote —
a repo already in the manifest just gains a host. The manifest is written sorted
and atomically, so two machines editing different repos merge cleanly in git.

## Manifest

Repoman uses a TOML manifest at `~/.config/repoman/manifest.toml`:

```toml
[defaults]
root = "~/src"

[[repos]]
name = "my-project"
remote = "git@github.com:user/my-project.git"
path = "~/src/my-project"
hosts = ["oleander", "gatekeeper"]
tags = ["work"]
status = "active"
```

Each repo lists which `hosts` it belongs on. `repoman status` only shows repos
for the current machine. Omit `path` and the repo resolves to `<defaults.root>/<name>`,
which keeps an entry portable across machines with different roots.

`REPOMAN_CONFIG` overrides the config directory; `REPOMAN_HOST` overrides the
detected hostname (useful for dry runs and testing).

**Pin the host name.** macOS renames a Mac to `oleander-5` after a Bonjour name
collision, which would file that machine's repos under a second identity. Run
`repoman host oleander` once per machine; the pin lives in `<config>/host` and
is excluded from the shared repo. `--from <old>` moves entries already filed
under the wrong name.

### Sharing the manifest

`~/.config/repoman` is itself a git repo pointed at a private remote.
`repoman sync-manifest` rebases onto it, commits local changes, and pushes.
Attaching a machine that already has a local manifest sets the local copy aside
as `manifest.local.toml` rather than clobbering either side — re-run
`repoman init` to fold this host's repos into the shared one.

## Architecture

```text
┌────────────────────────────────────────┐
│            git remotes                 │
│      (github, bitbucket, gitlab)       │
└──────────┬─────────────┬──────────────┘
           │             │
  ┌────────▼────┐ ┌──────▼──────┐
  │  machine A  │ │  machine B  │  ...
  │  manifest   │ │  manifest   │
  │  (subset)   │ │  (subset)   │
  └────────┬────┘ └──────┬──────┘
           │  restic      │
  ┌────────▼──────────────▼──────┐
  │   restic repo on a disk you  │
  │   own (always-on host), or   │
  │   S3-compatible storage      │
  │   snapshot-based, immutable  │
  └──────────────────────────────┘
```

**Key principle:** no continuous sync. Git handles code sync. Restic handles backup. The manifest tracks what goes where.

## Roadmap

- [x] `repoman init` — scan and generate manifest (merges, never clobbers)
- [x] `repoman status` — parallel repo status, `--problems`, other-host view
- [x] `repoman clone` — clone + register, or clone everything missing
- [x] `repoman pull` — parallel git pull
- [x] `repoman assign` / `unassign` — move repos between hosts
- [x] Multi-machine manifest sync (`sync-manifest`)
- [ ] Restic integration (backup/archive/restore) — repo on an always-on host
- [ ] Bundles (repos + `~/projects` assets as one unit)
- [ ] TUI (bubbletea)

## License

MIT
