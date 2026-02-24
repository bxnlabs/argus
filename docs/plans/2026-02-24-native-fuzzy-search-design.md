# Design: Native Go Fuzzy File Search

**Date**: 2026-02-24
**Status**: Approved

## Problem

File search depends on the external `fd` binary, which:
1. Doesn't support `!` negation in ignore files (silently ignores negation patterns)
2. Adds an external dependency that must be installed separately
3. Requires subprocess overhead on every search request

The `~/.argus/ignore` file uses `*` + `!Dir/` allowlist patterns that `fd --ignore-file` cannot honor.

## Solution

Replace `fd` with a pure Go implementation using three libraries:
- **[charlievieth/fastwalk](https://github.com/charlievieth/fastwalk)** — parallel directory walker (~2.5x faster than `filepath.WalkDir` on macOS)
- **[sahilm/fuzzy](https://github.com/sahilm/fuzzy)** — fuzzy string matching optimized for filenames (Sublime/VSCode-style scoring, ~30ms for 60K files)
- **[sabhiram/go-gitignore](https://github.com/sabhiram/go-gitignore)** — gitignore parser with full `!` negation and `**` glob support

Additionally, anchor search to the appropriate root directory based on UI context.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Fuzzy library | `sahilm/fuzzy` (1.4k stars) | Optimized for filenames/code symbols, VSCode-style scoring, zero external deps, returns match positions |
| Gitignore parser | `sabhiram/go-gitignore` (160 stars, 1,251 dependents) | Most widely adopted Go gitignore lib, supports `!` negation + `**` globs |
| Directory walker | `charlievieth/fastwalk` (120 stars) | 2.5x faster than stdlib on macOS, parallel `getdirentries`, actively maintained |
| Caching | Walk fresh every time | Simpler, always reflects latest filesystem state, fast enough with proper ignoring |
| fd availability endpoint | Remove | No external dependency = always available; drop `GET /api/files/search/available` |
| Debounce | Keep 300ms | Standard for search-as-you-type, sufficient with native Go speed |

## Architecture

```
Request → Walk → Filter → Collect → Fuzzy Match → Return
```

1. **Walk**: `fastwalk.Walk()` traverses from `searchDir` with parallel I/O
2. **Filter**: `go-gitignore` checks each entry against `~/.argus/ignore`
   - Ignored directories: return `filepath.SkipDir` (prunes entire subtree)
   - Ignored files: skip silently
   - Depth exceeds `maxDepth` (8): skip
3. **Collect**: Non-ignored paths accumulated into a slice (capped at 100K entries)
4. **Match**: `fuzzy.Find(query, basenames)` fuzzy-matches against basenames
   - Results sorted by match quality (exact > boundary > adjacent > scattered)
5. **Return**: Top N results wrapped in `FileSearchResponse`

## Search Anchoring

The search root varies by UI context:

| Context | Anchor | How |
|---------|--------|-----|
| Directory picker (new session dialog) | `$HOME` | Don't send `path` param (current default) |
| Attachments dialog (in a session) | Session `working_directory` | Pass `path={session.working_directory}` |
| Attachments dialog (no session) | `$HOME` | Don't send `path` param |

The backend already supports the `path` query parameter. The frontend needs to pass it when a session working directory is available.

## Code Changes

### Backend (`internal/agent/filesearch/`)

| File | Action |
|------|--------|
| `exec.go` | **Delete** — fd runner no longer needed |
| `exec_test.go` | **Delete** |
| `operations.go` | **Rewrite** — replace fd invocation with walk + filter + fuzzy match |
| `operations_test.go` | **Rewrite** — test walk/match behavior |
| `types.go` | **Keep** — same `FileSearchResult` / `FileSearchResponse` |

### Backend (`internal/agent/api/`)

| File | Action |
|------|--------|
| `files.go` | **Modify** — remove `/available` endpoint and `IsAvailable()` references |

### Frontend (`web/src/`)

| File | Action |
|------|--------|
| `data/files/queries.ts` | **Modify** — add optional `searchPath` param to `useFileSearchQuery`, include as `path` in URL params |
| `components/FileBrowser.tsx` | **Modify** — accept optional `searchPath` prop, pass to query hook |
| `components/FilePicker/index.tsx` | **Modify** — pass session `working_directory` as `searchPath` when available |

## API Surface

### Unchanged
- `Search(searchDir, query, searchType string, limit int) (*FileSearchResponse, error)`
- `GET /api/files/search?q=...&type=...&limit=...&path=...`

### Removed
- `IsAvailable() bool`
- `GET /api/files/search/available`

## New Dependencies

```
github.com/charlievieth/fastwalk
github.com/sahilm/fuzzy
github.com/sabhiram/go-gitignore
```

## Removed Dependencies

- External `fd` binary

## Preserved Behaviors

- `ensureIgnoreFile()` — unchanged, creates `~/.argus/ignore` with defaults on first use
- Type filtering — `searchType` "file"/"directory"/"" applied during walk via `DirEntry.IsDir()`
- Timeout — context with 5s deadline; on timeout, return collected results (graceful partial)
- Case-insensitive matching — `sahilm/fuzzy` handles natively
- Max depth — 8 levels, enforced during walk by counting path separators relative to root

## Limitations

- No incremental/cached index — walks filesystem on every request (fast enough with ignore pruning)
- No file content search — matches filenames only (same as current behavior)
