# View Session Info (BXN-97)

## Overview

Argus can list sessions but offers no way to inspect a single session in
detail, and the attached profile is invisible everywhere it matters. This adds
three things, all reading data that already exists:

1. A CLI command `argus session describe <id-or-name>` that prints a curated,
   sectioned summary of one session (with a `--json` escape hatch for scripting).
2. A read-only web info dialog, opened from the existing per-session `...`
   dropdown, showing the same curated fields.
3. The attached profile surfaced in two list views that currently hide it: the
   CLI `session ls` table and the web sidebar session row.

No new API endpoints, no data-model changes. `GET /api/sessions/{id}` already
returns the full `db.Session` record; the web already holds the `Session`
object and the runtime status map.

## Goals

- `argus session describe <id-or-name>` resolves by exact name then ID-prefix
  (same rules as other session subcommands) and prints a curated, grouped
  human-readable summary by default.
- `--json` on `describe` prints the raw session record (the `GET
  /api/sessions/{id}` body), pretty-printed, for piping to `jq`.
- A web info dialog shows the same curated fields, opened from a new "Session
  info" item in the existing session-row dropdown.
- The attached profile (when set) is visible in `session ls` and on each web
  sidebar row.
- CLI and web stay consistent on *which* fields count as the curated set.

## Non-Goals

- No new or changed API endpoints; no changes to the `Session` data model.
- No right-click context menu in the web app (the existing dropdown is the
  single entry point).
- No `--json` flag on any command other than `describe`.
- No editing from the info dialog — it is read-only (profile changes stay in
  the separate Change Profile dialog).
- No heavy web component tests; the web pieces are presentational and verified
  in the browser, per repo precedent.

## Curated field set

The single source of truth for *what data exists* is the `Session`
struct/type. "Curated" is a presentation choice applied per surface. Fields
shown (when present), grouped:

- **Header**: name, ID, runtime status (active/idle/dead), pinned, profile.
- **Provider**: provider type, model, auto-approve.
- **Location**: working directory (home-compressed), repo (parsed from
  `git_remote_url`), worktree branch.
- **Timestamps**: created, updated — absolute plus relative ("8 days ago").

Hidden as internal plumbing: `tmux_name`, `provider_session_id`,
`system_prompt`, `branch_created`, `last_viewed_at`, `unread_since`,
`user_marked_unread_at`. (These remain visible via `--json`.)

## Architecture

Independent rendering per surface. Each surface formats the curated fields in
its own medium — Go text for the CLI, a React dialog for the web — reading from
data each already has. There is no shared "describe payload" or backend
formatting endpoint: presentation differs by medium, and adding a backend
contract for it would be machinery with no payoff. This mirrors how `session
ls` already works (fetch raw, format locally).

Rejected alternative — **backend-provided describe payload** (an endpoint or
extended GET returning pre-curated/pre-formatted data both consume): more
moving parts, and it does not help because the terminal and the modal format
differently regardless.

## CLI — `argus session describe <id-or-name>`

New file `cmd/argus/cli/session_describe.go`, registered in `NewSessionCmd()`
(`cli.go`) alongside the other subcommands. `Args: cobra.ExactArgs(1)`,
`cmd.SilenceUsage = true`.

Flow:

1. Resolve the query to a session via `fetchAndResolve(c, query)` (`resolve.go`)
   — exact name, then ID-prefix, with the existing ambiguity error. This yields
   the session ID.
2. `c.get("/api/sessions/{id}")` for the full record. Decode the `session`
   object into a struct rich enough for the curated set (notably
   `git_remote_url`, which the lightweight `sessionInfo` lacks today — add the
   field to `sessionInfo`, or decode into a fuller local struct).
3. Best-effort `c.get("/api/sessions/status")` for live status, exactly as
   `session ls` does (don't fail if unavailable; show `-`).

Default output: curated sections built with existing helpers — `compressPath`
for the directory, `relativeTime` for timestamps, and the same remote-URL repo
parsing used elsewhere. Illustrative shape:

```
Session: bxn-97-session-info
  ID:        a1b2c3d4
  Status:    idle
  Pinned:    yes
  Profile:   default

Provider
  Type:         claude
  Model:        claude-opus-4-7
  Auto-approve: on

Location
  Directory: ~/Workspace/repos/bxnlabs/argus
  Repo:      bxnlabs/argus
  Branch:    jeev/bxn-97-session-info

Timestamps
  Created:   2026-05-20 14:32 (8 days ago)
  Updated:   2026-05-28 09:15 (just now)
```

`--json` flag: skip the curated formatter and the status call; pretty-print the
raw `session` object from the `GET /api/sessions/{id}` body to stdout.

## CLI — profile in `session ls`

In `session_list.go`, add a `PROFILE` column to the tabwriter table after
`PROVIDER`. Empty cell when no profile is attached. `sessionInfo` already
carries `Profile *string`, so no resolve.go change is needed for this part.

## Web — info dialog

New component `web/src/components/SessionInfoDialog/index.tsx`, modeled on
`ChangeProfileDialog`:

- Props: `session: Session | null`, `status?: string` (runtime status string),
  `homeDir: string`, `onClose: () => void`.
- Controlled by `open={session !== null}`, closes via `onOpenChange`.
- Read-only: a `DialogHeader` with the session name, the curated fields in
  grouped definition-list sections, and a single Close button (no Apply).
- Reuses web formatting helpers (`compressPath`, `formatRelativeTime`,
  `parseRepoFromRemoteURL`) so the field set matches the CLI.

Wiring in `SessionList/index.tsx`:

- Add a `"Session info"` `DropdownMenuItem` to `SessionItem` (lucide `Info`
  icon), near the top of the menu.
- Thread an `onViewInfo(session: Session)` callback through `SessionItem` and
  `SessionList` props, mirroring `onChangeProfile`.
- The parent that owns `ChangeProfileDialog`'s open-state owns the
  `SessionInfoDialog` state too, passing the matching `sessionStatuses[id]`
  status string and `homeDir`.

## Web — profile line in sidebar

In `SessionItem` (`SessionList/index.tsx`), add a metadata line after the
branch line, rendered only when `session.profile` is set: `SlidersHorizontal`
icon + profile name, styled like the existing directory/branch lines.

## Data flow summary

- CLI describe: `/api/sessions` (resolve) → `/api/sessions/{id}` (full record)
  → `/api/sessions/status` (best-effort live status). `--json` uses only the
  second call.
- CLI ls: unchanged calls; just renders the already-present `profile` field.
- Web dialog + sidebar: no new fetches — the `Session` object comes from the
  existing `useSessionsQuery` cache and the status from the sidebar's
  `sessionStatuses` map.

## Testing

- Go: table-driven unit test for the curated formatter and for resolution
  (name vs ID-prefix vs ambiguous), following existing CLI test style; cover the
  `--json` path and the optional/nil fields (no profile, no branch, no repo).
- Web: presentational; verify in the browser — dropdown opens the dialog,
  fields render correctly, the dialog is read-only, and the sidebar profile
  line appears only when a profile is attached. Add component tests only if the
  repo already has precedent for them.

## Out of scope

- New API endpoints or data-model changes.
- Right-click context menu.
- `--json` on commands other than `describe`.
- Any write/edit affordance in the info dialog.
