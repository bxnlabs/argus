---
name: argus
description: Driving Argus node sessions from the CLI. Use when a request mentions Argus (or the `argus` command) and wants to spawn a headless coding-agent session (Claude, Codex, Gemini, or a shell), send it prompts or control keys, poll its status, read its tmux output, read comments, or manage Argus git worktrees — e.g. "spin up an argus session", "peek at my argus session", "clean up dead argus sessions", "argus worktree". This skill is Argus-specific: do not use it for generic background jobs, plain tmux/ssh sessions, GitHub PR reviews, in-context research subagents, or raw `git worktree` when Argus isn't involved.
---

# Driving Argus sessions headlessly

Argus runs coding-agent sessions (Claude, Codex, Gemini, or a plain shell) as
tmux panes managed by a background node daemon. The `argus` CLI is a thin
client of that daemon. This skill drives a session end-to-end without ever
attaching to an interactive terminal.

## 1. Preflight

Confirm the node is up and reachable:

```bash
argus session ls
```

If this errors, the node daemon is not running. The node is a **long-running
process the user starts** — don't try to start it inline: bare `argus` runs the
server in the **foreground** and blocks forever, which will hang you. Surface
this to the user (ask them to start `argus`, or to point `$ARGUS_HOME` at the
right node) and stop.

Every command needs a local discovery file (`node.json`) pointing at a running
node, and in practice you run the CLI **on the node's host**:

- `peek`/`send` shell out to `tmux` against the node's socket.
- `git comments` and `git wt` resolve the repo from your current directory, and
  `comments` reads its comment data from local Argus state on disk — so run them
  inside the repo, on the node's host.
- `session new --src <path>` treats a local path as local to the node's
  filesystem. Only a remote git URL/shorthand source is portable across hosts.

`session new`/`ls`/`rm`/`describe`/`pwd` talk to the node over HTTP but still
require the local discovery file above.

## 2. Referencing a session

`send`, `peek`, `describe`, `pwd`, and `rm` all accept a **name or ID**. Capture
the ID when you spawn (below) and thread it through — IDs are unambiguous,
whereas a name can collide with another session.

## 3. Spawn (headless)

`session new` is headless by default and prints the bare session ID to stdout
(the full human-readable session summary goes to stderr), so capture it
directly:

```bash
id=$(argus session new my-task --src . --provider claude)
```

- `--src` is a local path or git URL/shorthand (defaults to the current dir).
  A local path is resolved on the node's filesystem as-is — there is no
  home-directory restriction. A git repo source gets an isolated worktree;
  a non-repo path (or any `shell` session) just runs in that directory.
- `--provider` is one of `claude` (default), `codex`, `gemini`, `shell`. Use
  `shell` when you just want to drive a shell (send a command, read its output)
  rather than a coding agent.
- **Auto-approve matters for headless.** Tool calls are auto-approved by default
  (`--yolo`). If you pass `--yolo=false`, the agent will stall on approval
  prompts that only a human — or you, via `send` — can answer, so keep the
  default unless you intend to babysit those prompts.
- `--branch <name>` sets an exact branch name (bypasses Argus's prefix/slug
  generation) — useful when you want a deterministic branch for automation.
- `--profile <name>` selects a profile for lifecycle hooks.
- `--attach` opens an interactive tmux instead (not for headless use); `--json`
  prints the full session record instead of the bare ID. They are mutually
  exclusive.

## 4. Drive

Send a prompt and submit it:

```bash
argus session send "$id" "Refactor the auth module and add tests" --enter
```

- Without `--enter` the text is pasted but **not** submitted. `--enter`
  delivers Return as a separate, slightly delayed write so the agent's TUI
  registers a submit instead of a newline in the prompt.
- For large or multi-line prompts, prefer a file (`-f/--file <path>`) or stdin
  (pipe) over an inline arg — the input is bracketed-pasted, so it's safe, but
  you skip all the shell-quoting pain.
- Send control keys with `--keys` (input is interpreted as tmux key names),
  e.g. interrupt the agent: `argus session send "$id" "C-c" --keys`. `--keys`
  also honors `--enter`, which appends a trailing `Enter`.

## 5. Observe

For a **single session**, `describe` is the handiest readout — it reports the
runtime status plus provider/model, auto-approve, directory, branch, and
timestamps:

```bash
argus session describe "$id"          # human-readable summary (includes Status)
argus session describe "$id" --json   # raw session record — metadata only
```

Note `--json` prints the stored session record (provider, directory, branch,
auto-approve, …) but **not** the runtime status — status is computed separately
and only appears in the human-readable `describe` and in `ls`. So parse the
human output (or `ls`) when you need status; use `--json` for stable metadata.

To **enumerate** sessions, use `ls`. It prints a wide table
(`ID PINNED NAME STATUS PROVIDER PROFILE DIRECTORY BRANCH UPDATED`); the STATUS
column is `active`, `idle`, `dead`, or `-` when the watcher has no snapshot yet,
and a leading `*` marks an unread session.

```bash
argus session ls
```

Read the pane with `peek`:

```bash
argus session peek "$id" --tail 50    # last 50 visible lines
```

- `peek` prints the currently visible pane. Use `--all` for the full scrollback
  history, `--head N`/`--tail N` to slice, and `-o <file>` to write to a file.
  A small `--tail N` can come back **blank** when the pane has trailing empty
  padding below the cursor — if you get nothing, widen N or use `--all`.
- `argus session pwd "$id"` prints the session's working directory — handy for
  locating or reading the files it produced.

### Knowing when a session is done

This is the crux of headless driving, and status alone won't tell you. The
`active`/`idle` status tracks whether the pane's **output is changing**, not
whether the underlying process is alive — a session that's quietly working (an
agent thinking, or a shell mid-`sleep`) reads `idle` even though it isn't
finished. So treat `idle` as "worth looking at now," then decide from the pane
itself:

- `peek` and look for a concrete completion marker: a returned shell prompt, the
  agent's end-of-turn output, or a sentinel you had the command print (end a
  shell command with `; echo __DONE__` and match a **standalone** `__DONE__`
  line). Anchor the match to a whole line so the echoed command text doesn't
  register as a false positive.
- Because a fast command can finish between polls, don't rely on catching an
  `active`→`idle` transition; poll the pane content until it both stabilizes and
  shows your marker.
- `dead` is not terminal: `peek` and `send` transparently revive a dead tmux
  session before acting, so you can keep driving it.

## 6. Comments (read-only)

Comments are left by a human (or another tool) in the Argus web UI;
from the CLI you read them for the current branch:

```bash
argus git comments ls      # compact table: ID, FILE:LINE, SUBMITTED, BODY
argus git comments view    # submitted comments rendered as markdown
```

Run these from inside the repo/worktree. Use `--base <branch>` to compare
against a base other than the detected default. There is no CLI command to
create or delete comments — that happens in the web UI.

## 7. Worktrees

Manage isolated worktrees for the current repo:

```bash
cd "$(argus git wt co feature-x)"   # create/reuse a worktree, cd into it
argus git wt ls                     # list managed worktrees (BRANCH, PATH)
argus git wt rm feature-x           # remove the worktree (branch is kept)
argus git wt rm feature-x --force   # remove even with uncommitted changes
```

`wt co` prints the worktree path to stdout precisely so you can `cd` into it —
a binary cannot change your shell's directory for you. Worktrees are created
under Argus's state dir regardless of where the repo lives, so any git repo on
the node's filesystem works.

## 8. Cleanup

```bash
argus session rm "$id"                    # delete the session
argus session rm "$id" --delete-branch    # also request branch deletion
```

Add `--force` to delete even when the worktree has uncommitted changes.

`--delete-branch` only deletes the branch when this is the last session on an
Argus-created branch, and it is best-effort — check the command's output to
confirm whether the branch was actually removed.

## Command reference

| Command | Purpose |
|---|---|
| `argus session ls` | List all sessions (wide table; STATUS is `active`/`idle`/`dead`/`-`) |
| `argus session new <name> --src <path> --provider <p>` | Create a headless session; prints its ID. Also `--yolo=false`, `--branch`, `--profile`, `--json`, `--attach` |
| `argus session describe <id> [--json]` | Status + provider/model/dir/branch/timestamps for one session |
| `argus session send <id> "<text>" --enter` | Paste text and submit it |
| `argus session send <id> "<keys>" --keys [--enter]` | Send tmux key names (e.g. `Escape`, `C-c`) |
| `argus session peek <id> [--tail N \| --head N \| --all] [-o file]` | Read the session's tmux contents |
| `argus session pwd <id>` | Print the session's working directory |
| `argus session rm <id> [--delete-branch] [--force]` | Delete a session |
| `argus git comments ls [--base <b>]` | List comments (table) |
| `argus git comments view [--base <b>]` | Show submitted comments as markdown |
| `argus git wt co <branch>` | Create/reuse a worktree; prints its path |
| `argus git wt ls` | List managed worktrees |
| `argus git wt rm <branch> [--force]` | Remove a worktree (branch preserved; `--force` discards uncommitted changes) |

## Gotchas

- **Don't run bare `argus` to "start the node."** It blocks in the foreground.
  The node is a daemon the user runs; if `session ls` fails, surface it.
- **Headless is the default.** `session new` prints the ID; use `--attach` only
  for interactive use.
- **Submitting needs `--enter`.** Without it, text is pasted but left unsent.
- **`--yolo=false` stalls headless runs.** Keep auto-approve on unless you plan
  to answer approval prompts yourself via `send`.
- **`peek`/`send` are host-local.** They require running on the node's host.
- **`idle` ≠ done.** Status tracks output churn, not process liveness — a quiet
  agent or a `sleep` reads `idle` while still working. Confirm completion from a
  marker in the pane (`peek`), not the status field.
- **`peek --tail N` can return blank.** Trailing pane padding below the cursor
  eats small N; widen it or use `--all`.
- **`describe` beats scraping `ls`** for one session's metadata, and `--json`
  gives a clean record to script against — but `--json` omits runtime status;
  read status from the human-readable `describe`/`ls` output.
- **`--all` for history.** The default `peek` only shows the visible pane.
- **Comments are read-only from the CLI.** Create them in the web UI.
- **`wt co` prints a path for `cd`.** Wrap it: `cd "$(argus git wt co <branch>)"`.
