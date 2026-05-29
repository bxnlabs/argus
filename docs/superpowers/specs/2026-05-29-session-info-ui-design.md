# Improve Session Info UI (BXN-104)

## Overview

The web session-info dialog and sidebar entries are functional but plain. This
is a presentation pass over both surfaces: restructure the info dialog into a
clean header + caption + copyable-field layout, introduce provider brand logos
shared by the dialog and the sidebar, and make the location paths/refs
click-to-copy.

No backend, API, or data-model changes. Everything reads data the web already
holds (the `Session` object from `useSessionsQuery` and the runtime status from
the sidebar's `sessionStatuses` map).

The "git changes" metric (lines added/deleted) from the original issue is
**explicitly punted** — see Non-Goals.

## Goals

- Info dialog header is always left-aligned (no center-on-mobile), shows a pin
  icon next to the name when the session is pinned, and shows the provider as a
  brand logo to the right of the name.
- A caption line under the name shows `status · relative-updated-time ·
  profile`, in the same visual format as the sidebar status line. Hovering the
  relative time reveals the full created and updated timestamps.
- Location values (directory, repo, branch, worktree directory) render as
  copyable monospace boxes with a copy affordance and toast/checkmark feedback.
- The sidebar session rows get the same provider brand logo, to the right of
  the name.
- The Provider "Type" text row, the Auto-approve field, and the Timestamps
  section are removed from the dialog (their information is now the logo, gone,
  and the caption tooltip respectively).

## Non-Goals

- **No git-changes metric** (lines added/deleted) in the dialog or sidebar.
  Computing per-session diff stats means a git operation per session — cheap for
  one dialog but an N-calls-per-poll cost for the sidebar — and the payoff did
  not justify the machinery for this pass. Dropped entirely.
- No backend, API, or data-model changes.
- No re-introduction of an inline pin icon in the **sidebar** rows. Commit
  `97ef53a` deliberately removed it in favor of the "Pinned" group header; the
  pin-next-to-name requirement applies to the dialog only.
- No editing from the dialog — it stays read-only.
- No new heavy web component test suites; presentational pieces are verified in
  the browser, per repo precedent. Pure helpers keep their unit tests.

## Decisions (resolved during brainstorming)

- **Git metric:** punted (above).
- **Provider representation:** brand logo SVGs (not generic lucide icons or text
  badges), in brand colors.
- **Location field mapping:** `Directory` = `git_parent_dir ?? working_directory`
  (main repo root), `Repo` = parsed from `git_remote_url`, `Branch` =
  `worktree_branch`, `Worktree dir` = `working_directory` shown **only** for
  worktree sessions (i.e. when `git_parent_dir` is set and differs from
  `working_directory`). This avoids a redundant duplicate box for plain
  sessions. Matches the CLI `describe` directory mapping.
- **Dialog body layout:** "copyable field stack" — labeled section headers
  (`DETAILS`, `LOCATION`) with location values as monospace code boxes.

## New shared pieces

### `web/src/components/ProviderLogo.tsx`

A small presentational component: `<ProviderLogo type={ProviderType}
className?=... />`. Renders an inline single-path brand SVG per provider:

- `claude` — Anthropic mark
- `codex` — OpenAI mark
- `gemini` — Google Gemini mark
- `shell` — lucide `Terminal` (already a dependency)
- unknown/missing — falls back to `Terminal`

The brand path data is vendored inline from simple-icons (single `<path>` per
mark) — no new npm dependency, just the path strings and brand hex colors.
Logos render in brand color. Sizing via `className` (e.g. `h-4 w-4` in the
dialog title, `h-3.5 w-3.5` in the sidebar). `aria-label` carries the provider
name for accessibility.

### `web/src/lib/sessionStatus.ts`

Extract the three status helpers currently private to `SessionList`
(`getStatusColor`, `getStatusLabel`, `getStatusAnimation`) into a shared module
so the dialog caption matches the sidebar status line exactly. `SessionList`
imports them from here instead of defining them locally. (`getStatusLabel`
returns `""` for unknown; the dialog caption falls back to "Unknown" the way the
current dialog does.)

### `web/src/lib/clipboard.ts`

`copyToClipboard(text: string): Promise<boolean>` using the existing pattern
from `Terminal/hooks/terminal-init.ts`: prefer `navigator.clipboard.writeText`,
fall back to a hidden-textarea + `document.execCommand("copy")`. Returns whether
the copy succeeded so callers can show success/failure feedback.

### `web/src/components/SessionInfoDialog/CopyableField.tsx`

A labeled "code box": a `<label>` line plus a bordered, monospace, wrapping
value with a copy button at the right. On click it calls `copyToClipboard`,
briefly swaps the copy icon for a check icon, and fires a `sonner` toast
(`toast.success("Copied")`). Props: `label`, `displayValue` (what is shown,
e.g. tilde-contracted path), `copyValue` (what is copied; defaults to
`displayValue`). Lives in the dialog folder — the dialog is its only consumer
(no premature globalization).

A lighter inline variant is needed for the `ID` row (value + copy icon on one
line, no box). This is a small prop/flag on `CopyableField` (e.g. `inline`) or a
second tiny component in the same file — implementation's choice; keep it
minimal.

## Info dialog redesign

### `web/src/components/SessionInfoDialog/fields.ts`

Replace `buildSessionInfoSections(): InfoSection[]` with a function that returns
a typed view-model the component renders directly:

```ts
interface SessionInfoModel {
  name: string;
  pinned: boolean;
  providerType: ProviderType;
  status: string | undefined;        // raw runtime status
  updatedRelative: string;           // formatRelativeTime(updated_at)
  createdAbsolute: string;           // raw created_at (for tooltip)
  updatedAbsolute: string;           // raw updated_at (for tooltip)
  profile: string | null;
  details: { id: string; model: string | null };
  location: {
    directory: { display: string; copy: string };
    repo: string | null;
    branch: string | null;
    worktreeDir: { display: string; copy: string } | null; // worktree only
  };
}
```

Path fields carry both a `display` value (tilde-contracted via `contractTilde`,
not truncated — the dialog has room and `break-all` wraps it) and a `copy` value
(the full absolute path). Branch and repo copy verbatim. The function stays pure
and keeps a unit test.

`fields.test.ts` is rewritten to assert the new view-model: directory mapping
(parent vs working dir), worktreeDir present only for worktree sessions,
optional repo/branch/model omitted when absent, profile null-handling.

### `web/src/components/SessionInfoDialog/index.tsx`

Header (`DialogHeader` given `className="text-left"` so it is left-aligned on
all breakpoints; no change to the shared `dialog.tsx`):

- Title row inside `DialogTitle`: a flex row — `[Pin icon if pinned]` +
  name (`truncate`, flex-1) + `<ProviderLogo>` (`flex-shrink-0`) pushed right.
- Caption (a `DialogDescription` or sibling div): status dot (color/animation
  from `sessionStatus.ts`) + status label · `updatedRelative` · profile (the
  profile segment omitted when null). The `updatedRelative` text is wrapped in
  the existing `Tooltip` (the app already has a `TooltipProvider`), whose
  content shows `Created: {createdAbsolute}` and `Updated: {updatedAbsolute}`.

Body:

- **DETAILS** section header + `ID` (inline copyable) and `Model` (plain text,
  rendered only when present).
- **LOCATION** section header + `CopyableField` boxes: `Directory`, `Repo` (if
  present), `Branch` (if present), `Worktree dir` (worktree sessions only).

Removed vs today: the Provider "Type" text row (now the logo), the Auto-approve
row, the Timestamps section (now the caption tooltip), and the standalone
Status / Pinned / Profile rows (now in the header and caption).

Props are unchanged (`session`, `status`, `homeDir`, `onClose`), so `App.tsx`
wiring is untouched.

## Sidebar parity — `web/src/components/SessionList/index.tsx`

- Import the status helpers from `web/src/lib/sessionStatus.ts` (delete the
  local copies).
- In `SessionItem`, the name line becomes a flex row: name (`truncate`, flex-1)
  + `<ProviderLogo>` (`flex-shrink-0`) at the right edge. All other metadata
  lines (status, directory, branch, profile) are unchanged.
- No pin icon added (see Non-Goals).

## Data flow

No new fetches. The dialog and sidebar both read the `Session` object already in
the React Query cache and the runtime status already in `sessionStatuses`. The
provider logo and copyable fields are pure functions of that data.

## Testing & verification

- `fields.test.ts`: rewritten table-driven coverage of the new view-model
  (directory mapping, worktree-only worktreeDir, optional fields, profile null).
- `SessionList/index.test.tsx`: keep `partitionSessions` / `readMenuState`
  tests; add a light assertion that a provider logo renders for a row.
- Type-check and lint clean.
- Browser verification (web dev server): dialog header left-aligned with pin +
  logo; caption tooltip reveals timestamps; copy buttons copy and toast; sidebar
  rows show the logo. Verify across providers (claude/codex/gemini/shell) and
  for a plain session (no branch/worktree) vs a worktree session.
- **tmux safety:** production argus runs on this machine on the default tmux
  socket with live sessions. Verification views existing sessions only — no
  creating, attaching, killing, or otherwise mutating real sessions, and no
  `tmux kill-server`.
