# Worktree & Git Repo Support Design

**Date:** 2026-02-25
**Status:** Approved

## Overview

Add support for running sessions in isolated git worktrees, and for launching sessions directly from remote git repos. Today, sessions require a pre-existing local directory. After this change, any session launched from a git repo (local or remote) automatically gets its own isolated worktree. Remote repos can be specified by URL or GitHub shorthand and are cloned on demand.

## Goals

- Sessions launched from a git repo always run in an isolated worktree
- Remote repos can be specified as `org/repo`, `https://...`, or `git@...`
- Remote repos are cloned to `~/.argus/projects/<parent>/gitrepo` and reused across sessions
- Worktrees are stored at `~/.argus/projects/<parent>/worktrees/<slug>`
- Branch names are configurable via `~/.argus/config.toml`
- Non-git directories continue to work exactly as today
- New `argus session pwd` command prints a session's working directory

## Non-Goals

- `argus worktree` subcommands for managing worktrees (manual cleanup only for now)
- Worktree deletion on session delete
- Support for non-GitHub shorthand (`org/repo` always means github.com)
- Real-time validation in the new session UI

---

## Section 1: Source Resolution

The `--dir` flag on `argus session new` is renamed to `--src`. It accepts:

- A local path (absolute, relative, or `~`-prefixed)
- A remote git URL: `git@github.com:org/repo.git` or `https://github.com/org/repo.git`
- A GitHub shorthand: `org/repo`

**Resolution order:**

1. Expand `~` and make absolute
2. If the path **exists on disk as a directory** → treat as local source
3. Otherwise → attempt to parse as a git URL or `org/repo` shorthand
4. If neither → error: `"not a valid path or git URL: <value>"`

Default (no `--src`) remains the current working directory, resolved as a local path.

---

## Section 2: Storage Layout & Config

```
~/.argus/
├── agent.db                                  # existing
├── agent.json                                # existing
├── config.toml                               # NEW
└── projects/
    ├── --Users--jeevb--Workspace--repos--bxnlabs--argus/
    │   └── worktrees/
    │       └── jeev--my-session/
    └── github.com--org--repo/
        ├── gitrepo/                          # cloned repo
        └── worktrees/
            └── jeev--my-session/
```

**Parent key conventions:**

- **Local repos:** `--` + git root abspath with `/` replaced by `--`
  e.g., `/Users/jeevb/repos/argus` → `--Users--jeevb--repos--argus`
- **Remote repos:** `<host>--<org>--<repo>`
  e.g., `github.com/bxnlabs/argus` → `github.com--bxnlabs--argus`

**`~/.argus/config.toml`:**

```toml
branch_prefix = "jeev"   # prepended to all worktree branches as <prefix>/<slug>
                         # omit or leave empty for no prefix
```

---

## Section 3: Data Model

One new nullable column added to the `sessions` table:

| Column | Type | Description |
|---|---|---|
| `worktree_branch` | `TEXT NULL` | Git branch name (e.g., `jeev/my-session`). Non-null signals a git-backed session. |

`working_directory` continues to hold the session's actual cwd. For git-backed sessions it equals the worktree path. No separate `worktree_path` column is needed.

The `Session` Go struct and JSON API gain the `worktree_branch` field (nullable string).

---

## Section 4: Session Lifecycle

### `argus session new <name> --src <value>` flow

```
1. Resolve source
   ├── Expand ~ / make absolute
   ├── Path exists on disk? → local source (step 2)
   └── Otherwise → parse as git URL (step 3)
       ├── "org/repo"                → https://github.com/org/repo.git
       ├── "https://..." / "git@..." → parse host/org/repo
       └── None match → error

2. Local source
   ├── git rev-parse --show-toplevel
   │   ├── Inside git repo →
   │   │   ├── parent key = "--" + root with / → --
   │   │   ├── worktree dir = ~/.argus/projects/<parent>/worktrees/<slug>
   │   │   ├── branch = <prefix>/<slug>  (or just <slug> if no prefix)
   │   │   ├── git worktree add <dir> -b <branch> <default-branch>
   │   │   └── working_directory = worktree dir
   │   └── Not a git repo → plain dir, working_directory = source path

3. Remote source
   ├── parent key = <host>--<org>--<repo>
   ├── clone dir = ~/.argus/projects/<parent>/gitrepo
   ├── Already cloned? → fetch + reset to default branch HEAD
   ├── Not cloned? → git clone <url> <clone-dir>
   ├── worktree dir = ~/.argus/projects/<parent>/worktrees/<slug>
   ├── branch = <prefix>/<slug>
   ├── git worktree add <dir> -b <branch> <default-branch>
   └── working_directory = worktree dir

4. Session created with working_directory + worktree_branch (NULL for plain dirs)
```

### Branch slug generation

Session name → lowercase → non-alphanumeric runs replaced with `-` → trim leading/trailing `-`.

Examples:
- `"Fix Auth Bug!"` → `fix-auth-bug`
- `"  my feature  "` → `my-feature`

### Branch conflict resolution

If `<prefix>/<slug>` already exists as a git branch, append `-2`, `-3`, etc. until unique.

### Default branch detection

Run `git symbolic-ref refs/remotes/origin/HEAD` or inspect remote refs. Fall back to `main`, then `master`, then error.

---

## Section 5: CLI Changes

### `--dir` → `--src`

The `--dir` flag on `argus session new` is renamed to `--src`. Behavior for local paths is unchanged.

### New command: `argus session pwd <name-or-id>`

Prints the session's `working_directory` to stdout. Useful for shell integration:

```zsh
# Add to ~/.zshrc or ~/.bashrc
acd() { cd "$(argus session pwd "$1")"; }
```

### Updated `argus session ls`

Add a `BRANCH` column (nullable). Shows the worktree branch for git-backed sessions, empty for plain sessions.

```
ID              NAME          STATUS   PROVIDER  BRANCH              UPDATED
sess_2x5a8b...  fix-auth      Idle     claude    jeev/fix-auth       2 min ago
sess_9k1p3c...  plain-shell   Idle     shell                         1 hr ago
```

---

## Section 6: UI Changes (New Session Dialog)

The "Working Directory" field is replaced by a **Local / Remote tab switcher**.

**Local tab** (default):
- Label: `Directory`
- Placeholder: `~/projects/my-app`
- Folder picker button retained for browsing
- Behavior: same as today; git detection happens server-side

**Remote tab:**
- Label: `Repository`
- Placeholder: `org/repo  or  https://github.com/org/repo.git`
- No folder picker
- Future-friendly: tab can grow to include repo search, recently cloned repos, etc.

**API change:** `CreateSessionParams.working_directory` → `CreateSessionParams.source` (optional string).

**Error display:** inline error below the input on session creation failure (same pattern as today).

---

## Section 7: Error Handling

| Scenario | Behavior |
|---|---|
| Path doesn't exist and isn't a valid git URL | Error: `"not a valid path or git URL: <value>"` |
| Clone fails (not found, no permissions, no network) | Surface git stderr; clean up any partially-created dirs |
| `git worktree add` fails | Surface git error |
| Default branch detection fails | Fall back to `main`, then `master`, then error |
| `config.toml` missing | Use defaults (`branch_prefix = ""`) |
| `config.toml` malformed | Warn and use defaults; do not fail session creation |
