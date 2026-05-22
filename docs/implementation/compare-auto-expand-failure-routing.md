# Compare view: auto-expand failure routing

**Status:** Implementation reference (design rationale)
**Area:** `web/src/components/GitPanel/CompareView.tsx`, `web/src/hooks/useExpandableDiff.ts`, `web/src/components/DiffViewer/{ExpandableUnifiedDiff,LazyFileDiff}.tsx`, `web/src/lib/compare-comments.ts`

This document explains how review comments are routed to their on-screen home in the
branch-compare view, why the "failure routing" machinery exists, and why its complexity
is warranted. It exists so we stop re-litigating the same design every time someone reads
it fresh and concludes it's over-engineered. **Read this before "simplifying" it.**

## The promise

A submitted review comment must **never silently disappear**. Whatever happens to the
underlying code, every comment renders *somewhere* the reviewer can see it.

## The three buckets

`partitionComments` (`compare-comments.ts`) classifies each comment against the **original
parsed diff hunks** into:

- **anchored** — the comment's line is present in a diff hunk. Render it inline. Done.
- **caseB** — the line still *exists in the file* but isn't shown by the diff. (A diff only
  renders changed lines plus a little context; a comment can sit on a real line that's far
  from any change.) The line is real, so routing it to "unanchored" would be a lie — we want
  it inline.
- **unanchored** — the code is genuinely gone. There's no honest inline location, so the
  comment goes in a dedicated read/prune section.

## Auto-expand, and why it can fail

For a caseB comment, the target line exists but isn't on screen. The view performs
**auto-expand**: it fetches the surrounding lines from the server and splices them into the
diff as a synthetic context hunk (`useExpandableDiff.expandToLine` → `INSERT_HUNK`), giving
the comment a visible line to attach to.

Auto-expand can **fail**:

- the requested window overlaps an existing hunk but still doesn't cover the target line
  (overlap-without-coverage),
- the range is empty / past EOF,
- the fetch errors (404 / 413 / 422 / network).

When it fails, the caseB comment has **no inline home**. If nothing acts on that, the comment
vanishes — breaking the promise. **Failure routing is the safety net that detects this and
moves the comment into the visible unanchored section instead.** That is its entire reason to
exist; it is not optional.

## Why the work is split between parent and child

- The **child** (`useExpandableDiff`, hosted in `ExpandableUnifiedDiff`, one per file) is the
  only component that actually runs the expand, so it is the only thing that *knows* whether a
  given comment ended up covered.
- The **parent** (`CompareView`) renders the single, cross-file, sorted **unanchored section**.
  So it must *know which comments failed* in order to pull them out of the per-file inline
  lists and place them in that shared section.

Hence the round-trip: **child detects failure → reports up
(`onAutoExpandFailuresChange`) → parent re-buckets (`effectivePartition`) → renders them in
the unanchored section.** Most of the apparent complexity is just this transport, and exists
because the *detector* and the *renderer of the fallback* are different components.

## The load-bearing reason the decision lives in the reducer (the race)

This is the part that justifies the careful design. **Do not move routing out of the reducer.**

Auto-expand is asynchronous: it requests lines, `await`s, then splices them in. **While it is
waiting, the user can manually expand** a nearby region (`EXPAND_UP` / `EXPAND_DOWN`), which
also mutates the hunks. By the time the auto-expand's lines return, the hunk set may have
shifted.

If "did this comment get covered?" were decided from a snapshot taken *before* the `await`
(or from `expandToLine`'s returned boolean), the answer could be wrong — it would be judging
against stale state. So all hunk mutations are funneled through the **reducer**, which applies
them one at a time. Only the reducer, at the moment it applies a change, sees the *true
current* hunks and can correctly classify each anchor (`INSERT_HUNK` / `FAIL_ANCHORS` →
`recordFailures`). That is why:

- the failure decision lives in the reducer's `failedAnchors`, not in an outer post-`await`
  check, and
- `expandToLine`'s return value is explicitly **non-authoritative** (the child ignores it; it
  observes `failedAnchors`).

## Healing, and full-set reporting

A failure is not necessarily permanent. If the user later manually expands and that growth
covers a previously-failed anchor, the comment should return inline ("heal"). This is why:

- the child reports its **full current** failed set (a set-replacement), not "add this one"; the
  set can **shrink**, and
- `reconcileFailures` drops an anchor from `failedAnchors` once a hunk mutation covers it.

The retained `commentId → line` mapping is what makes healing possible — `reconcileFailures`
needs the line to test coverage.

## The ref-not-state performance choice

Expanded hunks are reported to the parent into a **ref** (`expandedHunksRef`), deliberately
**not** React state. If they were state, every expand (including the common success case)
would re-render the parent. And any design that *also* derived routing from expanded hunks
(as the rejected "derive from the hunk set" rewrite does) would then re-run
`partitionComments` over **every file** on every expand. Today `partitionComments` depends
only on `parsedDiffs`, `totalLines`, and `comments` — not on expanded hunks — so the ref
keeps large diffs fast while the failure set (which changes rarely) flows through state.

## What is warranted vs. what is merely clerical

Inherent to the problem (keep these):

1. **Detect failure and reroute** — or comments vanish.
2. **Decide it in the single serialized reducer** — or the async race produces wrong answers.
3. **Let failures heal** — or a manual expand can't bring a comment back.

Reducible (clerical) — the parent-side mirror `failedByFile` + the `failedCommentIds` union +
the `effectivePartition` re-derivation. This is just transport carrying the child's answer up
to the shared list. It can be collapsed, but it is not the hard part and carries real
regression surface, so it is **not** worth churning on its own.

## Rejected "simplifications" (and why)

Two rewrites are tempting and both are **worse**:

- **"Derive inline-vs-unanchored purely from the current expanded hunk set."** Forces
  `expandedHunksRef` into state → re-partition on every expand (perf regression). Worse, it
  *cannot represent* failures that produce no hunk at all (EOF / empty / fetch error): a
  hunk-only derivation can't tell "uncovered because still pending" from "uncovered because it
  terminally failed," so those comments never route to unanchored.
- **"Track placement from `expandToLine` promise outcomes in the parent."** Routes from the
  explicitly non-authoritative boolean and reintroduces the exact race the reducer closes — a
  manual expand committing between fetch-return and observation flips coverage.

Both trade warranted complexity for actual bugs.

## Lifecycle invariant: failed files stay mounted

`LazyFileDiff` lazy-mounts via `useLazyMount`, which is **sticky** — once a file scrolls into
view, `shouldMount` latches `true` and never reverts. But `forceMount` can mount a file
*without* latching `shouldMount` (`mounted = forceMount || shouldMount`).

A file with a caseB comment is force-mounted via `commentsByFile.has(pathKey)` so its
auto-expand can run — even while off-screen. If that expand **fails**, the comment moves to
the unanchored section and the file drops out of `commentsByFile`. If it also never scrolled
into view, `shouldMount` is still `false`, so it would **unmount**. On a later remount the
child reinitializes `failedAnchors` to empty and reports `[]`, which the parent reads as "all
healed" — bouncing the comment back to caseB with no covering hunk, a flicker where it renders
nowhere before re-failing.

The fix: include `failedByFile.has(pathKey)` in the `forceMount` condition so a failed file
**stays mounted**, preserving its reducer's `failedAnchors` (and the line `reconcileFailures`
needs). The `failedByFile` reset effect also keys on `headRef`/`baseRef`, because the diff
`key` remounts children on a refs change while `parsedDiffs` can stay reference-stable for a
byte-identical diff.

### Known residual

Any remount of the whole diff subtree from *outside* `LazyFileDiff` is not covered by the
`forceMount` fix — notably switching the **mobile** diff ↔ file-list view, and desktop↔mobile
breakpoint flips (the two render different top-level trees). On such a transition a failed
comment can still flicker once. It **self-corrects** (the remounted child re-derives the
failure via a re-fetch; no data is lost). Fixing it robustly would require hydrating the child
reducer's initial `failedAnchors` from the parent — threading state through three layers —
which adds to the very surface this document argues against expanding. Left as a documented
limitation unless review of off-diff caseB comments through those transitions becomes a real
workflow.
