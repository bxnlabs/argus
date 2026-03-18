# Inline Comments on Compare View

**Date:** 2026-03-18
**Branch:** jeev/git-review-panel
**Status:** Draft

## Summary

Add inline code commenting to the git panel's Compare view, enabling humans to leave precise, line-level feedback on committed diffs. Comments are stored as a JSON file outside the repo and can be retrieved by AI agents via the `argus` CLI to act on the feedback.

This is not a formal review system. It is a lightweight annotation primitive — the human leaves text comments on code, and the agent reads and addresses them.

## Motivation

When an AI agent makes code changes in Argus, the human currently has no way to give precise, line-anchored feedback. They must describe issues in natural language in the terminal ("on line 47 of auth.ts, change the expiry to 3600"), hoping the agent interprets correctly.

The open-source project [linemark](https://github.com/gdaybrice/linemark) demonstrates a browser-based diff review UI for Claude Code where users click lines, leave comments, and submit structured feedback. Argus already has a Compare view showing branch diffs — we can add commenting directly to it.

## Design Principles

- **GitHub-familiar UX** — Click a line number, type a comment, submit. Developers already know this flow.
- **Ephemeral by nature** — Comments live for the lifecycle of a branch comparison. No long-term storage, no database.
- **Pull-based agent integration** — The agent fetches comments when ready via CLI. No real-time injection, no push notifications.
- **Snippet-anchored** — Every comment captures the actual code text at the commented lines. This serves two purposes: helping the agent find the right code regardless of line shifts, and detecting staleness when the code changes.

## User Flow

1. Agent works on a branch, makes commits.
2. Human opens Git panel > Compare tab (branch vs base).
3. Human clicks a line number in the diff — inline comment form slides in below that line.
4. Human types a comment, clicks "Add comment" (or Cmd+Enter).
5. Comment card renders inline in the diff. Repeat across files.
6. A comment summary bar appears at the bottom with a comment count, a general comment textarea, and a "Submit comments" button.
7. Human clicks "Submit comments" — all pending comments are marked as submitted and persisted.
8. Human tells the agent to check feedback, or agent uses a skill that runs `argus comments get`.
9. Agent reads the structured markdown output and acts on the comments.
10. Agent makes new commits addressing the feedback.
11. Human opens Compare tab again — staleness detection runs automatically:
    - Comments whose snippets no longer exist in the file are pruned (agent touched that code).
    - Comments whose snippets still match are re-anchored to their current line numbers and shown inline.
12. Human verifies fixes, leaves new comments if needed. Repeat.

## Data Model

### Storage Location

```
~/.argus/projects/<projectKey>/comments/<branch>--<baseBranch>.json
```

Comments are stored outside the repository, alongside other Argus project data. No `.gitignore` needed. Scoped to the branch comparison — any session working on the same branch sees the same comments.

### File Structure

```json
{
  "branch": "feat/auth-system",
  "baseBranch": "main",
  "comments": [
    {
      "id": "rc_1710583800_a3f2",
      "file": "src/auth.ts",
      "oldLine": { "from": 47, "to": 47 },
      "newLine": { "from": 52, "to": 52 },
      "snippet": "const TOKEN_EXPIRY = 1800;",
      "body": "Token expiry should be 3600 not 1800",
      "submitted": true,
      "createdAt": "2026-03-16T10:30:00Z"
    },
    {
      "id": "rc_1710583860_b7d1",
      "file": "src/auth.ts",
      "oldLine": { "from": 12, "to": 15 },
      "newLine": { "from": 12, "to": 15 },
      "snippet": "function validateToken(token) {\n  if (!token) return false;\n  return token.exp > Date.now();\n}",
      "body": "This doesn't check token signature",
      "submitted": false,
      "createdAt": "2026-03-16T10:31:00Z"
    }
  ],
  "generalComment": {
    "body": "Auth looks mostly good, but token handling needs hardening",
    "submitted": true,
    "createdAt": "2026-03-16T10:32:00Z"
  }
}
```

### Field Descriptions

| Field | Description |
|-------|-------------|
| `id` | Timestamp + random suffix, generated client-side |
| `file` | File path relative to repo root |
| `oldLine` | Line range in the base version (from diff parser) |
| `newLine` | Line range in the branch version (from diff parser) |
| `snippet` | Actual code text at the commented lines — used for agent context and staleness detection |
| `body` | The comment text |
| `submitted` | `false` = draft (not yet sent), `true` = submitted |
| `createdAt` | ISO 8601 timestamp |
| `generalComment` | Optional cross-cutting comment not tied to any specific line |

## Staleness Detection

Staleness is computed server-side on every `GET /api/git/comments` request. It is never stored as a field.

For each submitted comment:

1. Read the current content of `comment.file` from disk.
2. Search for `comment.snippet` as a substring in the file content.
3. **Found** — the comment is still relevant. Compute the new line number from the match position and update `newLine`.
4. **Not found** — the agent changed that code. Remove the comment from the array.

Write the pruned/updated array back to the file before returning to the frontend.

**Edge case:** Whitespace-only reformatting (e.g., prettier) will cause snippets to not match, removing the comment. This is acceptable — the human re-reviews and re-comments if needed. Better to over-prune than show stale comments on wrong lines.

## Backend API

### `GET /api/git/comments`

- Reads the comments file for the current branch comparison.
- Runs staleness detection (snippet matching, pruning, re-anchoring).
- Returns cleaned comment data as JSON.

### `POST /api/git/comments`

- Accepts the full comments state (comments array + general comment) from the frontend.
- Writes to the comments file.
- Called on comment add, comment delete, and submit (with `submitted` flags flipped).

### `DELETE /api/git/comments`

- Deletes the comments file.
- Used to clear all comments for a branch comparison.

## Frontend Changes

### UnifiedDiff.tsx

**Clickable line numbers.** Line number cells in the diff table get a hover state (background highlight, pointer cursor). Clicking a line number opens an inline comment form below that row.

**Multi-line selection.** Click a line number, then shift-click another line in the same file to select a range. The range highlights and the comment form anchors below the last line in the range.

**Inline comment form.** Renders as a new row spanning the full diff table width, below the selected line. Contains:
- Auto-growing textarea (placeholder: "Leave a comment...")
- "Add comment" button (primary) and "Cancel" button (ghost)
- Keyboard shortcuts: Cmd+Enter to add, Escape to cancel

**Inline comment cards.** After adding, the comment renders as a card in the same row position. Contains:
- Comment body text
- Delete button (x icon)
- Submitted comments get a subtle visual indicator
- Cards are visually distinct from diff content (light border, slightly different background)

### Comment Summary Bar

Appears anchored to the bottom of the Compare view when unsubmitted comments exist. Contains:
- Comment count: "3 pending comments"
- General comment textarea (collapsible, placeholder: "General feedback...")
- "Submit comments" button (primary)

Disappears when there are no unsubmitted comments. When the user returns to Compare view after submitting, they see their submitted comment cards inline but no summary bar — until they add a new comment.

## Agent Interface

### CLI Command

```bash
argus comments get
```

- Resolves the current working directory to the project key.
- Determines the current branch and base branch.
- Reads the comments file.
- Outputs structured markdown to stdout.

### Example Output

```markdown
## Comments
Branch: feat/auth-system vs main

### src/auth.ts

**Lines 52-52:**
> const TOKEN_EXPIRY = 1800;

Token expiry should be 3600 not 1800

### src/auth.ts

**Lines 12-15:**
> function validateToken(token) {
>   if (!token) return false;
>   return token.exp > Date.now();
> }

This doesn't check token signature

### General

Auth looks mostly good, but token handling needs hardening
```

Markdown is the output format because the agent is an LLM — it reads markdown natively. Snippets are quoted for clear delineation. File paths and line numbers are explicit.

### Skill Integration

A skill instructs the agent to run `argus comments get` and treat the output as feedback to address. The skill is minimal — "run this command and act on the findings."

The agent does not write back to the comments file. The lifecycle is strictly: human writes, agent reads. Staleness detection handles cleanup automatically when the code changes.

## What Is NOT In Scope

- No review mode toggle or "Start Review" button
- No severity levels, categories, or comment types
- No agent acknowledgment or response contracts
- No approval/rejection states per file or hunk
- No real-time notifications or WebSocket push events
- No review rounds tracking — staleness detection handles this implicitly
- No persistence beyond the branch comparison lifecycle
- No image attachments
