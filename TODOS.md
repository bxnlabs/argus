# TODOs

## Status Detection

- [ ] Add structured/debug logging for silent error paths in `refreshCache` (detector.go:65) and `CapturePaneContext` (detector.go:221). Both discard errors, which is self-correcting but makes debugging tmux failures invisible. Address when introducing structured logging across the agent.
- [ ] Add tests for the spike detection state machine, cooldown transitions, and acknowledged/unacknowledged flows in `internal/agent/status/`. Requires injectable clock and tmux interface to test temporal logic without real subprocesses.
- [ ] Restructure `GetStatus` mutex scope to exclude tmux subprocess calls (`CapturePaneContext`, `refreshCache`). Currently the mutex is held during external I/O, serializing all status checks. Operationally fine at 1-10 sessions but will degrade beyond ~30. Fix by gathering tmux data outside the lock, then acquiring it only for in-memory state updates.
- [ ] Wire up `Detector.Acknowledge()` (`detector.go:254`). The method exists but is never called — no API endpoint, no frontend trigger. Without it, sessions that finish running stay yellow (waiting/unacknowledged) forever and never transition to grey (idle). Need to add a `POST /api/sessions/{name}/acknowledge` endpoint and call it from the frontend when the user views/selects a session.

## Terminal WebSocket

- [ ] Set `ws.SetReadLimit(64 * 1024)` on the WebSocket connection in `handleConnection` (`internal/agent/terminal/handler.go`). Gorilla's default read limit is effectively unlimited (`MaxInt64`). A misbehaving client on the tailnet could send a multi-GB frame and exhaust agent memory. One-line fix.
- [ ] Replace raw `err.Error()` in the PTY error path (`handler.go:190`) with a generic error message. Currently sends Go error strings (filesystem paths, binary names) to the WebSocket client. Log the detail server-side instead.
- [ ] Add connection counting or a semaphore to limit concurrent WebSocket-PTY sessions in `handleConnection` (`internal/agent/terminal/handler.go`). Each connection allocates a PTY fd pair + spawns a process. No limit means runaway reconnect loops could exhaust file descriptors (default 1024). Operationally fine at 1-10 sessions but needs a safety net (e.g., cap at 50) before wider use.

## Session UI

- [ ] When `onRenameSession` is wired to a real mutation, revisit the blur-to-confirm rename pattern in `SessionList/index.tsx`. Currently blur commits the rename AND the subsequent click navigates to the clicked session (standard Notion/Linear behavior). If user testing reveals confusion, add a zero-delay debounce guard (`renamingJustFinished` ref) so the click that caused the blur is suppressed.
- [ ] When `onDeleteSession` is wired to a real mutation, change its prop type from `void` to `Promise<void>` in `SessionListProps` and `await` each call in `handleBulkDelete`. Use `mutateAsync` (not `mutate`) from the TanStack Query hook so errors propagate. `SelectionToolbar`'s error handling is already sound (clears selection in `try`, not `finally`).

## Git Operations

- [ ] `GetFileContent` in `internal/agent/git/operations.go` masks all `git show HEAD:<file>` errors as "new file" (`isNew=true`). If git fails for another reason (corrupt object, permission denied, disk full), the caller silently gets wrong data. Distinguish "file not in HEAD" (exit 128 with `"does not exist in"` or `"exists on disk, but not in"`) from real errors.
- [ ] Add HTTP handler tests for `internal/agent/api/git.go`. The git package functions are well-tested, but the handler layer (query param parsing, error responses, path expansion) has zero coverage. Use `httptest.NewServer` with a temp git repo.
- [ ] `Check()` and `GetStatus()` in `internal/agent/git/operations.go` run 3-4 sequential git commands sharing a single context timeout (10s). If an early command is slow (e.g., network-backed repo), later commands get less remaining time and may spuriously fail. Not an issue on local repos. If remote/network-backed repos are ever supported, give each command its own derived context.
- [ ] Directory rename parsing in `GetCommitDetail` (`internal/agent/git/history.go:205`) only handles the simple `old => new` numstat format. The brace format `{old => new}/shared/path` (used for directory renames) extracts only the `new}/shared/path` suffix. Parse the brace format to reconstruct the full new path.
- [ ] `handleFileClick` in `GitPanel/index.tsx:58` has no request cancellation. Rapid clicks fire overlapping fetches; a slow response for file A can resolve after file B's and overwrite the displayed diff. Add an `AbortController` that cancels the in-flight request when a new file is clicked.
- [ ] Scrollbars in GitPanel history and diff viewer are always visible in Chrome but hidden in Safari. The `overflow-y-auto`/`overflow-auto` containers in `GitPanel/index.tsx` and `CommitHistory.tsx` use native browser scrolling with no custom scrollbar CSS. Chrome renders persistent classic scrollbars while Safari uses macOS overlay scrollbars that fade when idle. Replace native scroll divs with the existing Radix `ScrollArea` component (`web/src/components/ui/scroll-area.tsx`) for consistent cross-browser overlay scrollbar behavior.

## Deferred Features (Phase 2+)

- [ ] **Diff context expansion** — add GitHub-style expand buttons to the diff viewer so users can reveal surrounding code above/below hunks. Backend serves context line slices from a cached full-context diff (`git diff -U99999`), frontend splices them into the existing parsed diff via `useReducer`. See design: `docs/plans/2026-02-16-diff-context-expansion-design.md`.
- [ ] **Git write operations** — stage, unstage, discard, commit, push, PR creation. Phase 2 git panel is read-only. Add backend endpoints (`POST /api/git/stage`, `POST /api/git/commit`, etc.) and frontend components (`CommitForm`, `PRCreationModal`, `FileEditDialog`) in a future phase.
- [ ] **Multi-repo git aggregation** — sessions are currently flat with a single working directory. When multi-repo sessions are reintroduced, aggregate git status across multiple repositories within a session.
- [ ] **Worktree management** — create, delete, list, and configure git worktrees via API. No API routes existed in the old codebase (library only). Design the UI before implementing.

## Session Lifecycle

- [ ] `SendKeys` in `internal/agent/session/tmux.go:89` uses a fixed temp filename (`/tmp/argus-send-<name>.txt`) and fixed tmux buffer name per session. Concurrent calls can overwrite each other's payload or delete each other's buffer, sending incorrect commands. Use per-request unique temp files and unique buffer names, or serialize per-session sends with a mutex.
- [ ] `SendKeys` temp file path in `internal/agent/session/tmux.go:89` is predictable and written with `0644`. Vulnerable to symlink/clobber attacks and exposes keystroke payloads to other local users. Use `os.CreateTemp` with `0600`.
- [ ] Init script path in `internal/agent/session/initscript.go:62` is predictable per session ID and written with `0755`. Same symlink/clobber risk as SendKeys. Use `os.CreateTemp` with `0600`, then chmod only as needed, or write to a private directory.

## File Operations

- [ ] Oversized file uploads return `500` instead of `413` in `internal/agent/api/files.go:154`. `MaxBytesReader` errors become generic write errors. Detect `*http.MaxBytesError` via `errors.As` and return `StatusRequestEntityTooLarge`.
- [ ] New-file writes in `internal/agent/files/operations.go:106` are forced to `0644` via `chmod`, which can bypass stricter process umask expectations. For new files, respect umask or use a secure default (e.g., `0600`).

## API Security

- [ ] Revisit `CleanPath` filesystem-wide access scope. File browse/read/write handlers use `CleanPath` (no home-directory restriction), relying on OS permissions and private network (Tailscale) as the sole access guards. If the agent is ever exposed beyond private networks, add application-level auth or scope restrictions to write operations (`writeContent` in `internal/agent/api/files.go`).

- [ ] Replace `Access-Control-Allow-Origin: *` with a scoped CORS policy. The wildcard header is set globally in the API router and allows any origin to make requests to the agent. While the agent runs on a private tailnet, tightening CORS prevents a malicious page from issuing authenticated requests to `localhost:<port>` if the user visits it in the same browser.
- [ ] Bind the agent API to `127.0.0.1` by default instead of all interfaces (`:3011`). Combined with the wildcard CORS and no auth, this exposes file read/write, session control, and terminal attach to the network. Add authentication (token/session) before binding to non-loopback interfaces.
- [ ] `/api/code-search` in `internal/agent/api/search.go:30` uses `ExpandPath` instead of `SafeExpandPath`, allowing callers to search arbitrary host paths (e.g., `/etc`). Use `shared.SafeExpandPath` to enforce the same root policy as other file endpoints.
- [ ] User query in `internal/agent/filesearch/operations.go:56` is appended to `fd` args without a `--` separator. Queries starting with `-` are parsed as flags, enabling option injection (`--search-path`, `--exec`). Append `--` before the query arg.

## Pre-Release Hardening

- [ ] Enable `noUnusedLocals: true` and `noUnusedParameters: true` in `web/tsconfig.json` (lines 16-17). Currently disabled to reduce noise during active development. Flip before phase 1H (integration testing) to catch dead code and refactoring oversights at compile time.
- [ ] Remove unused CSS theme variables from `web/src/globals.css`: `chart-1` through `chart-5` (both `@theme` mappings and `.dark` values) are shadcn/ui scaffolding with zero consumers. Prune unused `sidebar-*` variants (keep `sidebar-background`, `sidebar-foreground` which are in use).
