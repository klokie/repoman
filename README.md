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

Or build from source:

```bash
git clone https://github.com/klokie/repoman.git
cd repoman
go build -o repoman ./cmd/repoman
```

## Quick start

```bash
# scan existing repos and generate manifest
repoman init

# see what you've got
repoman status
```

## Commands

| Command | Description |
| --- | --- |
| `repoman init` | Scan ~/Sites + ~/src for git repos, generate `~/.config/repoman/manifest.toml` |
| `repoman status` | Show branch, dirty/clean, unpushed counts for all repos (parallel) |
| `repoman clone <url>` | Clone a repo and register it in the manifest *(planned)* |
| `repoman pull` | Git pull all active repos in parallel *(planned)* |
| `repoman archive <name>` | Backup local state, remove clone, mark archived *(planned)* |
| `repoman restore <name>` | Clone from remote + restore .env from restic *(planned)* |
| `repoman backup` | Restic snapshot of non-git files to Wasabi/S3 *(planned)* |

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

Each repo lists which `hosts` it belongs on. `repoman status` only shows repos for the current machine. Set `REPOMAN_CONFIG` to override the config directory.

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
  │     S3-compatible storage    │
  │   (wasabi, backblaze, etc)   │
  │   snapshot-based, immutable  │
  └──────────────────────────────┘
```

**Key principle:** no continuous sync. Git handles code sync. Restic handles backup. The manifest tracks what goes where.

## Roadmap

- [x] `repoman init` — scan and generate manifest
- [x] `repoman status` — parallel repo status
- [ ] `repoman clone` — clone + register
- [ ] `repoman pull` — parallel git pull
- [ ] Restic integration (backup/archive/restore)
- [ ] Multi-machine manifest sync
- [ ] TUI (bubbletea)

## License

MIT
