# Design: Global Search Ignore File

**Date**: 2026-02-24
**Status**: Approved

## Problem

File search (`fd`) traverses `$HOME` with `--hidden --max-depth 8`, which takes ~7-8 seconds on macOS due to `~/Library` and other heavy directories. Exclude patterns are hardcoded in `operations.go`, giving users no control over search scope.

## Solution

A user-editable ignore file at `~/.argus/search-ignore` using gitignore/fdignore glob syntax, passed to `fd` via its native `--ignore-file` flag.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| File format | gitignore syntax | Familiar, well-documented, fd consumes natively |
| Include/negation | Not supported | `fd --ignore-file` doesn't honor `!` negation; excludes-only covers the primary use case |
| Search roots | Not supported | Exclude patterns are sufficient; keeps the design simple |
| Hardcoded excludes | Removed | Ignore file is the single source of truth |
| File parsing | None | `fd --ignore-file <path>` handles everything |

## File Location & Format

**Path**: `~/.argus/search-ignore`

```gitignore
# Argus search ignore patterns (gitignore syntax)
# Edit this file to control which directories fd skips during search.
# See: https://git-scm.com/docs/gitignore#_pattern_format

# Version control
.git/

# Package managers & toolchains
node_modules/
.npm/
.nvm/
.cargo/
.rustup/

# IDE & editor state
.vscode/
.cursor/
.claude/

# Caches & local data
.cache/
.local/
.config/

# macOS
Library/
```

## Behavior

1. **First search**: If `~/.argus/search-ignore` doesn't exist, auto-create it with sensible defaults (above).
2. **Subsequent searches**: Pass `--ignore-file <path>` to every `fd` invocation.
3. **Empty file**: No excludes applied (user explicitly wants everything searched).

## Code Changes

- **`operations.go`**: Remove `hiddenExcludes` var and the `--exclude` loop in `buildFdArgs()`. Add `--ignore-file` flag.
- **New function** `ensureIgnoreFile(home string) (string, error)`: Creates default ignore file if missing; returns the path.
- **`Search()`**: Call `ensureIgnoreFile` before building fd args, pass ignore file path through.
- **Tests**: Update `buildFdArgs` tests for `--ignore-file` instead of `--exclude` flags. Add test for `ensureIgnoreFile`.

## Limitations

- `!` negation patterns in the ignore file are silently ignored by `fd --ignore-file`.
- No hot-reloading or file watching; fd reads the file on each invocation naturally.
