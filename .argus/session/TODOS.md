# Deferred Follow-ups — orphan-comments simplification

Surfaced by the final review of `docs/superpowers/plans/2026-05-15-simplify-orphan-comments.md`. Both are functionally correct today; tracked here as low-priority clean-up.

## 1. Dead `actualEnd < start` guard in `GetFileLines`

**File:** `internal/node/git/operations.go`, lines 599–602 (`getFileLinesFromDisk`) and 693–696 (`getFileLinesFromRef`).

```go
actualEnd := start + len(lines) - 1
if actualEnd < start {   // unreachable
    actualEnd = start
}
```

After Task 5's revert, the only path that produces `len(lines) == 0` is `start > totalLines`, which returns `ErrInvalidInput` before the `actualEnd` computation. The guard is dead code — leftover from the silent-clamp era walked back in commit `7a180a1`.

**Fix:** drop the `if` block in both backends; simplify to `actualEnd := start + len(lines) - 1`.
