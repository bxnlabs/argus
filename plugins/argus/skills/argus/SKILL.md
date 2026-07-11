---
name: argus
description: Use when you need to run, spawn, or drive Argus node sessions from the CLI — create headless coding-agent sessions, send them prompts, read their tmux output, poll their status, read review comments, and manage git worktrees, all without an interactive terminal.
---

# Driving Argus sessions headlessly

Argus runs coding-agent sessions (Claude, Codex, Gemini, or a plain shell) as
tmux panes managed by a background node daemon. The `argus` CLI is a thin
client of that daemon. This skill drives a session end-to-end without ever
attaching to an interactive terminal.

## The loop

### 1. Preflight

Confirm the node is up and reachable:

```bash
argus session ls
```

If this errors, the node daemon is not running — start it (`argus`) or point
`$ARGUS_HOME` at the right node first. `peek` and `send` shell out to `tmux`
against the node's socket, so they only work **on the same host as the node**.
`session new`/`ls`/`rm` and the `git` commands use HTTP and work from anywhere
the node is reachable.

### 2. Spawn (headless)

`session new` is headless by default and prints the bare session ID to stdout
(the human-readable "Created session" note goes to stderr), so capture it
directly:

```bash
id=$(argus session new my-task --src . --provider claude)
```

- `--src` is a local path or git URL/shorthand (defaults to the current dir).
- `--provider` is one of `claude` (default), `codex`, `gemini`, `shell`.
- Tool calls are auto-approved by default; pass `--yolo=false` to require
  approval.
- `--attach` opens an interactive tmux instead (not for headless use).
- `--json` prints the full session record instead of the bare ID.

### 3. Drive

Send a prompt and submit it:

```bash
argus session send "$id" "Refactor the auth module and add tests" --enter
```

- Without `--enter` the text is pasted but **not** submitted. `--enter`
  delivers Return as a separate, slightly delayed write so the agent's TUI
  registers a submit instead of a newline in the prompt.
- Input can also come from a file (`-f/--file <path>`) or stdin (pipe).
- Send control keys with `--keys` (input is interpreted as tmux key names),
  e.g. interrupt the agent: `argus session send "$id" "C-c" --keys`.

### 4. Observe

Poll status and read the pane:

```bash
argus session ls                     # STATUS column: active | idle | dead
argus session peek "$id" --tail 50   # last 50 visible lines
```

- `idle` means the watcher saw the pane's output stop — the agent is likely
  waiting for you. Poll `session ls` until the session is `idle`.
- `peek` prints the currently visible pane. Use `--all` for the full
  scrollback history, `--head N`/`--tail N` to slice, and `-o <file>` to write
  to a file instead of stdout.

### 5. Review comments (read-only)

Review comments are left by a human (or another tool) in the Argus web UI;
from the CLI you read them for the current branch:

```bash
argus git comments ls      # compact table: ID, FILE:LINE, SUBMITTED, BODY
argus git comments view    # submitted comments rendered as markdown
```

Run these from inside the repo/worktree. Use `--base <branch>` to compare
against a base other than the detected default. There is no CLI command to
create or delete comments — that happens in the web UI.

### 6. Worktrees

Manage isolated worktrees for the current repo:

```bash
cd "$(argus git wt co feature-x)"   # create/reuse a worktree, cd into it
argus git wt ls                     # list managed worktrees (BRANCH, PATH)
argus git wt rm feature-x           # remove the worktree (branch is kept)
```

`wt co` prints the worktree path to stdout precisely so you can `cd` into it —
a binary cannot change your shell's directory for you.

### 7. Cleanup

```bash
argus session rm "$id"                    # delete the session
argus session rm "$id" --delete-branch    # also delete its git branch
```

Add `--force` to delete even when the worktree has uncommitted changes.

## Command reference

| Command | Purpose |
|---|---|
| `argus session ls` | List sessions with `active`/`idle`/`dead` status |
| `argus session new <name> --src <path> --provider <p>` | Create a headless session; prints its ID |
| `argus session send <id> "<text>" --enter` | Paste text and submit it |
| `argus session send <id> "<keys>" --keys` | Send tmux key names (e.g. `Escape`, `C-c`) |
| `argus session peek <id> [--tail N \| --head N \| --all] [-o file]` | Read the session's tmux contents |
| `argus session rm <id> [--delete-branch] [--force]` | Delete a session |
| `argus git comments ls [--base <b>]` | List review comments (table) |
| `argus git comments view [--base <b>]` | Show submitted comments as markdown |
| `argus git wt co <branch>` | Create/reuse a worktree; prints its path |
| `argus git wt ls` | List managed worktrees |
| `argus git wt rm <branch>` | Remove a worktree (branch preserved) |

## Gotchas

- **Headless is the default.** `session new` does not attach; it prints the ID.
  Use `--attach` only for interactive use.
- **Submitting needs `--enter`.** Without it, text is pasted but left unsent.
- **`peek`/`send` are host-local.** They require running on the node's host.
- **`idle` ≠ done.** It means output stopped; the agent may be waiting for
  input. Read the pane with `peek` to decide the next prompt.
- **`--all` for history.** The default `peek` only shows the visible pane.
- **Comments are read-only from the CLI.** Create them in the web UI.
- **`wt co` prints a path for `cd`.** Wrap it: `cd "$(argus git wt co <branch>)"`.
