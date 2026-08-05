# Slack-style Compose Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the mobile compose zone as a rounded, inset card with a Slack-style `Message #<slug>` placeholder and ghost actions, at +1px of chrome.

**Architecture:** The compose panel is absolutely positioned and the key toolbar is in normal flow, so they cannot share a border box. The card is assembled from two halves that meet flush — the panel takes `rounded-t-lg border-x border-t`, the toolbar takes `rounded-b-lg border-x border-b` — and the seam between them is the row divider. The session slug is computed server-side by a new `internal/slug` leaf package and carried on every API response as a derived (non-column) field, so no slugify rule is ported to TypeScript.

**Tech Stack:** Go 1.26.3 (`github.com/bxnlabs/argus`), React 19 + TypeScript, Tailwind v4 with `tailwind-merge`, Vitest + Testing Library, `go test`.

## Global Constraints

- Mobile only. Every web change lands inside the `isMobile` branch of `web/src/components/Terminal/index.tsx`. Desktop is untouched.
- **The growth architecture does not change.** Spacer in flow, panel absolutely positioned against the spacer's bottom edge growing upward, terminal shifted by a transform. Growing the draft must never refit the terminal.
- **The spacer height must equal the panel's resting height.** If they disagree the panel permanently overflows, the `ResizeObserver` reports a nonzero overlay at rest, and the terminal carries a constant shift. Task 6 moves both together; the drift guard in `ComposeBar.test.tsx` is what enforces it.
- Card edge: `hsl(0 0% 20%)` at rest, `hsl(0 0% 30%)` focused. Row divider: `hsl(0 0% 14%)`. Surface: `bg-input`.
- Placeholder text: `hsl(0 0% 50%)` — 4.6:1 on the 8% card, the dimmest gray clearing the 4.5:1 text bar. Send glyph idle: `hsl(0 0% 45%)` — 3.86:1, clearing the 3:1 non-text bar. Attach glyph: `hsl(0 0% 60%)`. Key labels stay `hsl(0 0% 72%)`.
- Card margins: `mt-1.5` / `mb-1.5` (6px), horizontal inset `2` (8px). Panel padding `py-1`. Radius `lg` (10px).
- Slug truncation: 28 characters, via the existing `truncateRight` helper in `@/lib/utils`.
- Go tests: `go test ./internal/...`. Web tests: `cd web && pnpm test`. Web typecheck: `cd web && pnpm exec tsc -b` (silent on success).

---

### Task 1: Extract `internal/slug`

`slugify` currently lives unexported in `internal/git/worktree`. The session read path needs the same rule, and `internal/node/db` importing `internal/git/worktree` would invert the layering — so it moves to a leaf package that imports nothing but stdlib.

**Files:**
- Create: `internal/slug/slug.go`
- Create: `internal/slug/slug_test.go`
- Modify: `internal/git/worktree/slugify.go` (delete `slugify` and its regexp var)
- Modify: `internal/git/worktree/slugify_test.go` (delete `TestSlugify`)
- Modify: `internal/git/worktree/manager.go:209-216`

**Interfaces:**
- Consumes: nothing.
- Produces: `slug.Make(name string) string` at `github.com/bxnlabs/argus/internal/slug`.

- [ ] **Step 1: Write the failing test**

Create `internal/slug/slug_test.go`. This is the table lifted verbatim from `internal/git/worktree/slugify_test.go` — the behaviour must not change, only its address.

```go
package slug

import "testing"

func TestMake(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Fix Auth Bug!", "fix-auth-bug"},
		{"  my feature  ", "my-feature"},
		{"already-valid", "already-valid"},
		{"123abc", "123abc"},
		{"UPPER CASE", "upper-case"},
		{"multiple   spaces", "multiple-spaces"},
		{"a--b", "a-b"},
		{"!!!!", "session"},
		{"", "session"},
	}
	for _, tc := range cases {
		got := Make(tc.input)
		if got != tc.want {
			t.Errorf("Make(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/slug/...`
Expected: FAIL — `no required module provides package`, or `undefined: Make` once the directory exists.

- [ ] **Step 3: Write minimal implementation**

Create `internal/slug/slug.go`:

```go
// Package slug converts human-entered session names into identifiers safe for
// git branch names and for display.
//
// This rule began as an unexported helper in internal/git/worktree. It lives
// in its own leaf package because two unrelated layers need it: the worktree
// manager derives branch names from it, and the session read path exposes it
// on the API. Keeping it dependency-free is what lets internal/node/db use it
// without importing internal/git/worktree and inverting the layering.
package slug

import (
	"regexp"
	"strings"
)

var nonAlphanumRun = regexp.MustCompile(`[^a-z0-9]+`)

// Make converts a session name into a valid git branch name component.
// Lowercases, collapses non-alphanumeric runs to "-", trims leading/trailing
// "-". Returns "session" if the result is empty.
func Make(name string) string {
	lower := strings.ToLower(name)
	s := nonAlphanumRun.ReplaceAllString(lower, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "session"
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/slug/...`
Expected: `ok  	github.com/bxnlabs/argus/internal/slug`

- [ ] **Step 5: Delete the old copy and repoint its caller**

In `internal/git/worktree/slugify.go`, delete the `regexp` import, the `nonAlphanumRun` var, and the whole `slugify` function. The file keeps only `worktreeDirName` and its comment, so the import block becomes:

```go
package worktree

import (
	"strings"
)
```

In `internal/git/worktree/slugify_test.go`, delete `TestSlugify` entirely (the whole function including its cases table). `TestWorktreeDirName` stays untouched.

In `internal/git/worktree/manager.go`, add the import:

```go
	"github.com/bxnlabs/argus/internal/slug"
```

and replace the else branch at lines 209-216. Before:

```go
	var baseBranch string
	if branchOverride != "" {
		baseBranch = branchOverride
	} else {
		slug := slugify(sessionName)
		baseBranch = m.branchName(slug)
	}
```

After — inlined, because a local named `slug` would shadow the package for the rest of the function:

```go
	var baseBranch string
	if branchOverride != "" {
		baseBranch = branchOverride
	} else {
		baseBranch = m.branchName(slug.Make(sessionName))
	}
```

- [ ] **Step 6: Run both packages' tests**

Run: `go test ./internal/slug/... ./internal/git/worktree/...`
Expected: both `ok`. `TestWorktreeDirName` still passes; `TestSlugify` no longer exists in `worktree` and now runs as `TestMake` in `slug`.

- [ ] **Step 7: Verify no stale references remain**

Run: `grep -rn "slugify" internal/`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add internal/slug internal/git/worktree
git commit -m "refactor(slug): extract slugify into a dependency-free leaf package

The session read path needs the same rule the worktree manager uses, and
internal/node/db importing internal/git/worktree would invert the layering."
```

---

### Task 2: Carry the slug on the session payload

`db.Session` gains a field derived from `Name` on every read. `scanSession` is the only seam needed: `Create` re-fetches through `GetSession` after insert (`lifecycle.go:257`), `Update` returns `GetSession` (`lifecycle.go:647`) and `db.UpdateSession` additionally re-scans its own `RETURNING` row, and `Get`/`List` scan directly. The one `db.Session{}` literal (`lifecycle.go:232`) is insert-only and never marshalled.

Deriving rather than storing is also what makes rename correct for free — `PATCH /sessions/{id}` changes `Name`, and the next scan recomputes.

**Files:**
- Modify: `internal/node/db/models.go:14` (add field after `Name`)
- Modify: `internal/node/db/sessions.go:15-34` (`scanSession`)
- Test: `internal/node/db/db_test.go` (append)

**Interfaces:**
- Consumes: `slug.Make(name string) string` from Task 1.
- Produces: `db.Session.Slug string` with JSON tag `slug`, populated on every read path.

- [ ] **Step 1: Write the failing tests**

Append to `internal/node/db/db_test.go`:

```go
func TestGetSessionDerivesSlugFromName(t *testing.T) {
	db := testDB(t)

	s := &Session{
		ID:               "sess-slug-1",
		Name:             "Fix Auth Bug!",
		TmuxName:         "claude-sess-slug-1",
		WorkingDirectory: "~/code",
		ProviderType:     "claude",
	}
	if err := db.CreateSession(s); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetSession("sess-slug-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "fix-auth-bug" {
		t.Errorf("slug = %q, want %q", got.Slug, "fix-auth-bug")
	}
}

func TestUpdateSessionRecomputesSlug(t *testing.T) {
	// The slug is derived, not stored, so a rename must be reflected with no
	// migration and no recompute-on-write. This is the test that would fail
	// if someone later "optimised" it into a column.
	db := testDB(t)

	s := &Session{
		ID:               "sess-slug-2",
		Name:             "Old Name",
		TmuxName:         "claude-sess-slug-2",
		WorkingDirectory: "~/code",
		ProviderType:     "claude",
	}
	if err := db.CreateSession(s); err != nil {
		t.Fatal(err)
	}

	newName := "Brand New Name"
	updated, err := db.UpdateSession("sess-slug-2", SessionUpdate{Name: &newName})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Slug != "brand-new-name" {
		t.Errorf("slug after rename = %q, want %q", updated.Slug, "brand-new-name")
	}
}

func TestListSessionsCarriesSlug(t *testing.T) {
	db := testDB(t)

	s := &Session{
		ID:               "sess-slug-3",
		Name:             "UPPER CASE",
		TmuxName:         "claude-sess-slug-3",
		WorkingDirectory: "~/code",
		ProviderType:     "claude",
	}
	if err := db.CreateSession(s); err != nil {
		t.Fatal(err)
	}

	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found *Session
	for _, sess := range sessions {
		if sess.ID == "sess-slug-3" {
			found = sess
		}
	}
	if found == nil {
		t.Fatal("session not in list")
	}
	if found.Slug != "upper-case" {
		t.Errorf("slug = %q, want %q", found.Slug, "upper-case")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/db/ -run 'Slug' -v`
Expected: FAIL to compile — `s.Slug undefined (type *Session has no field or method Slug)`.

- [ ] **Step 3: Add the derived field**

In `internal/node/db/models.go`, insert immediately after the `Name` field:

```go
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	// Slug is derived from Name, not a column. scanSession computes it on
	// every read so all API responses carry it; CreateSession names its
	// columns explicitly and so never writes it. Deriving rather than
	// storing is what makes rename correct for free — PATCH changes Name
	// and the next scan recomputes, with no migration to backfill.
	Slug               string  `json:"slug"`
	TmuxName           string  `json:"tmux_name"`
```

- [ ] **Step 4: Populate it in the single scan seam**

In `internal/node/db/sessions.go`, add the import:

```go
	"github.com/bxnlabs/argus/internal/slug"
```

and set the field alongside the other post-scan conversions in `scanSession`:

```go
	s.AutoApprove = autoApprove != 0
	s.BranchCreated = branchCreated != 0
	s.Pinned = pinned != 0
	// Derived, not scanned — see the field comment in models.go. Doing it
	// here rather than at each call site is what guarantees every response
	// path carries it: Create and Update both re-read through GetSession.
	s.Slug = slug.Make(s.Name)
	return &s, nil
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/node/db/ -run 'Slug' -v`
Expected: all three PASS.

- [ ] **Step 6: Run the full Go suite for regressions**

Run: `go test ./internal/...`
Expected: all packages `ok`. The new field is additive and never written, so nothing else should move.

- [ ] **Step 7: Commit**

```bash
git add internal/node/db
git commit -m "feat(api): carry a derived session slug on every read path

Computed in scanSession from Name rather than stored, so rename stays
correct with no column and no migration."
```

---

### Task 3: Rename `Terminal`'s `sessionName` prop to `sessionId`

The prop has always received `getTabSessionId(tab)` — a session ID. Task 4 introduces a genuinely name-derived `sessionSlug` alongside it, and the two would read as near-duplicates. This is a pure rename with no behaviour change; the gate is the typechecker.

Out of scope: `SessionStatusInfo.sessionName` in `web/src/types.ts` is a different, unrelated field (and is never read anywhere). Leave it.

**Files:**
- Modify: `web/src/components/Terminal/index.tsx:31-32, 47, 67`
- Modify: `web/src/components/Terminal/hooks/useTerminalConnection.types.ts:14`
- Modify: `web/src/components/Terminal/hooks/useTerminalConnection.ts:23, 105-106, 133, 138, 213`
- Modify: `web/src/components/Terminal/hooks/websocket-connection.ts:27, 36, 252`
- Modify: `web/src/components/Workspace/index.tsx:258`

**Interfaces:**
- Consumes: nothing.
- Produces: `Terminal` prop `sessionId: string | null`; `useTerminalConnection` option `sessionId`; `openTerminalWebSocket`'s second parameter `sessionId`.

- [ ] **Step 1: Confirm the rename is scoped to exactly these five files**

Run:
```bash
cd web && grep -rn "sessionName" src/components/Terminal src/components/Workspace/index.tsx
```
Expected: hits only in the five files listed above. If any other file appears, stop and re-scope — `src/types.ts` and `src/data/statuses/` must NOT be touched.

- [ ] **Step 2: Apply the rename**

Run:
```bash
cd web && sed -i 's/sessionName/sessionId/g' \
  src/components/Terminal/index.tsx \
  src/components/Terminal/hooks/useTerminalConnection.types.ts \
  src/components/Terminal/hooks/useTerminalConnection.ts \
  src/components/Terminal/hooks/websocket-connection.ts \
  src/components/Workspace/index.tsx
```

This also rewrites the explanatory comments at `useTerminalConnection.ts:105-106` ("sessionId=null spawns a raw shell; sessionId=... attaches to session by ID") and `websocket-connection.ts:252`, which is intended — they describe this parameter.

- [ ] **Step 3: Verify no stale references and no collateral damage**

Run:
```bash
cd web && grep -rn "sessionName" src/components/Terminal src/components/Workspace/index.tsx
```
Expected: no output.

Run:
```bash
cd web && grep -rn "sessionName" src/types.ts
```
Expected: still one hit at `src/types.ts:21` — the unrelated `SessionStatusInfo.sessionName`. If this is gone, the sed was over-scoped; revert and redo.

- [ ] **Step 4: Typecheck and run the suite**

Run: `cd web && pnpm exec tsc -b`
Expected: silent (no output) on success.

Run: `cd web && pnpm test`
Expected: all pass. No test references this prop, so nothing should change.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Terminal web/src/components/Workspace/index.tsx
git commit -m "refactor(web): rename Terminal's sessionName prop to sessionId

It has always carried a session ID. The next commit adds a genuinely
name-derived sessionSlug alongside it."
```

---

### Task 4: `Message #<slug>` placeholder

One placeholder in both focus states, dimmed, naming the session. Replaces the `focused ? "Message…" : "Tap to compose"` swap.

**Files:**
- Modify: `web/src/types.ts:1-18` (add `slug`)
- Modify: `web/src/components/SessionList/index.test.tsx:5-9`, `web/src/components/SessionInfoDialog/index.test.tsx:36-40`, `web/src/components/SessionList/SessionItem.test.tsx:33-37`, `web/src/components/SessionInfoDialog/fields.test.ts:5-9`, `web/src/components/ChangeProfileDialog/index.test.tsx:58-62` (fixtures)
- Modify: `web/src/components/Terminal/ComposeBar.tsx`
- Modify: `web/src/components/Terminal/index.tsx`
- Modify: `web/src/components/Workspace/index.tsx:258`
- Test: `web/src/components/Terminal/ComposeBar.test.tsx:150-160`

**Interfaces:**
- Consumes: `Session.slug` from Task 2's API field; `truncateRight(s: string, max: number): string` from `@/lib/utils`.
- Produces: `ComposeBar` prop `sessionSlug?: string | null`; `Terminal` prop `sessionSlug?: string | null`.

- [ ] **Step 1: Write the failing tests**

In `web/src/components/Terminal/ComposeBar.test.tsx`, replace the existing test at lines 150-160 (`"swaps the placeholder between focused and unfocused"`) with these four:

```tsx
  it("names the session in the placeholder, Slack-style", () => {
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "mobile-persistent-input",
    });

    expect(textarea().placeholder).toBe("Message #mobile-persistent-input");
  });

  it("keeps one placeholder across focus and blur", () => {
    // Replaces the old focused/unfocused swap. Focus is now carried by the
    // card edge brightening alone, so the placeholder must hold still.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "argus",
    });

    expect(textarea().placeholder).toBe("Message #argus");
    fireEvent.focus(textarea());
    expect(textarea().placeholder).toBe("Message #argus");
    fireEvent.blur(textarea());
    expect(textarea().placeholder).toBe("Message #argus");
  });

  it("drops the channel when there is no session, as on a raw-shell tab", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    expect(textarea().placeholder).toBe("Message");
  });

  it("truncates a long slug rather than letting it clip mid-word at the input edge", () => {
    // A textarea placeholder cannot ellipsize itself. 36 chars is a real
    // session name from `argus session ls`.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "review-mike-dp-host-self-registration",
    });

    expect(textarea().placeholder).toBe("Message #review-mike-dp-host-self-re…");
    expect(textarea().placeholder.length).toBe("Message #".length + 28);
  });

  it("dims the placeholder without dimming the draft", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    expect(textarea().className).toContain("placeholder:text-[hsl(0_0%_50%)]");
    expect(textarea().className).not.toMatch(/(^|\s)text-\[hsl/);
  });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/components/Terminal/ComposeBar.test.tsx`
Expected: the five new tests FAIL — placeholder is `"Tap to compose"`, and `sessionSlug` is not a known prop (TS error in the test file).

- [ ] **Step 3: Add `slug` to the Session type and its fixtures**

In `web/src/types.ts`, add the field after `name`:

```ts
export interface Session {
  id: string;
  name: string;
  /** Server-derived from `name`; see internal/slug. Safe for display as a channel. */
  slug: string;
  tmux_name: string;
```

`slug` is required, so the five full-literal `makeSession` fixtures stop typechecking. Add a line to each, immediately after its `name:` line:

- `web/src/components/SessionList/index.test.tsx` → `slug: "name",`
- `web/src/components/SessionInfoDialog/index.test.tsx` → `slug: "my-session",`
- `web/src/components/SessionList/SessionItem.test.tsx` → `slug: "my-session",`
- `web/src/components/SessionInfoDialog/fields.test.ts` → `slug: "name",`
- `web/src/components/ChangeProfileDialog/index.test.tsx` → `slug: "my-session",`

(`web/src/hooks/useSessionDeepLink.test.ts` builds its fixture with `as Session` and needs no change.)

- [ ] **Step 4: Build the placeholder in ComposeBar**

In `web/src/components/Terminal/ComposeBar.tsx`, add the import:

```tsx
import { cn, truncateRight } from "@/lib/utils";
```

Add the module-level helper above the `ComposeBarProps` interface:

```tsx
// Longest slug that fits the input at 15px on a 390px-wide phone alongside
// both action glyphs. A textarea placeholder cannot ellipsize itself, so
// without this a long name simply clips mid-word at the input edge and reads
// as a bug. `review-mike-dp-host-self-registration` is a real session name.
const MAX_SLUG_CHARS = 28;

function composePlaceholder(slug?: string | null): string {
  if (!slug) return "Message";
  return `Message #${truncateRight(slug, MAX_SLUG_CHARS)}`;
}
```

Add the prop to `ComposeBarProps`:

```tsx
  /**
   * Server-derived session slug, rendered Slack-style as the destination.
   * Absent on a raw-shell tab, which has no session.
   */
  sessionSlug?: string | null;
```

Destructure it in the component signature alongside `workingDirectory`, then replace the textarea's `placeholder` line:

```tsx
              placeholder={composePlaceholder(sessionSlug)}
```

and add the placeholder colour to its className (the rest of the class string is unchanged):

```tsx
              className="w-full resize-none overflow-y-auto bg-transparent py-1.5 text-[15px] leading-[var(--compose-row-h)] placeholder:text-[hsl(0_0%_50%)] [grid-area:1/1/2/2] focus:outline-none"
```

- [ ] **Step 5: Thread the prop through Terminal and Workspace**

In `web/src/components/Terminal/index.tsx`, add to `TerminalProps` after `workingDirectory`:

```tsx
  /** Server-derived session slug, shown in the compose placeholder */
  sessionSlug?: string | null;
```

Add `sessionSlug` to the destructured parameter list, and pass it to `ComposeBar`:

```tsx
            <ComposeBar
              onSend={handleSend}
              connected={isConnected}
              workingDirectory={workingDirectory}
              sessionSlug={sessionSlug}
              onOverlayHeightChange={setComposeOverlay}
            />
```

In `web/src/components/Workspace/index.tsx`, add the prop to the `<Terminal>` element inside the `tabs.map` (immediately after `sessionId={getTabSessionId(tab)}`):

```tsx
                      sessionSlug={
                        sessions.find((s) => s.id === tab.sessionId)?.slug
                      }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd web && pnpm test src/components/Terminal/ComposeBar.test.tsx`
Expected: all PASS, including the five new tests.

- [ ] **Step 7: Typecheck and run the full suite**

Run: `cd web && pnpm exec tsc -b`
Expected: silent.

Run: `cd web && pnpm test`
Expected: all pass — the five fixture files included.

- [ ] **Step 8: Commit**

```bash
git add web/src/types.ts web/src/components
git commit -m "feat(web): name the session in the compose placeholder

One dim 'Message #<slug>' in both focus states, using the server-derived
slug. Focus is now carried by the card edge alone."
```

---

### Task 5: Ghost actions, permanent send

Slack's send is always present and never a filled circle. Making it permanent retires "Send appears" as a focus cue — with the placeholder swap already gone, the card edge carries focus by itself.

Both controls go ghost rather than following Slack literally (its `+` is filled): a filled attach beside a ghost send would invert the weight-follows-importance hierarchy established in the 2026-08-04 refresh.

**Files:**
- Modify: `web/src/components/Terminal/ComposeBar.tsx:64-67, 145-158, 202-216`
- Test: `web/src/components/Terminal/ComposeBar.test.tsx:136-148`

**Interfaces:**
- Consumes: `sessionSlug` prop from Task 4.
- Produces: no new interfaces. `showSend` is deleted.

- [ ] **Step 1: Write the failing tests**

In `web/src/components/Terminal/ComposeBar.test.tsx`, replace the test at lines 136-148 (`"hides the send button only when the bar is unfocused AND empty"`) with:

```tsx
  it("keeps the send button mounted at all times, as Slack does", () => {
    // Replaces the old focus/draft-gated visibility. A ghost glyph is quiet
    // enough at rest that hiding it buys nothing, and its disabled state now
    // carries "present, not yet available" on its own.
    renderComposeBar({ onSend: () => true, connected: true });

    const send = () => screen.getByRole("button", { name: /send/i });
    expect(send()).toHaveProperty("disabled", true);

    fireEvent.focus(textarea());
    expect(send()).toHaveProperty("disabled", true);

    fireEvent.change(textarea(), { target: { value: "draft" } });
    expect(send()).toHaveProperty("disabled", false);

    fireEvent.blur(textarea());
    // A draft must stay sendable after tapping away to the terminal.
    expect(send()).toHaveProperty("disabled", false);
  });

  it("renders both actions as ghost glyphs, with send the only coloured one", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const attach = screen.getByRole("button", { name: /attach/i });
    const send = screen.getByRole("button", { name: /send/i });

    // No filled circles: a filled attach beside a ghost send would make the
    // secondary action heavier than the primary one.
    expect(attach.className).not.toMatch(/\bbg-\[hsl/);
    expect(send.className).not.toMatch(/\bbg-(primary|\[hsl)/);

    expect(attach.className).toContain("text-[hsl(0_0%_60%)]");
    expect(send.className).toContain("text-primary");
    expect(send.className).toContain("disabled:text-[hsl(0_0%_45%)]");
  });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/components/Terminal/ComposeBar.test.tsx`
Expected: FAIL — `Unable to find an accessible element with the role "button" and name /send/i` on the first assertion, since send is unmounted while empty and unfocused.

- [ ] **Step 3: Delete `showSend`**

In `web/src/components/Terminal/ComposeBar.tsx`, delete these three lines (64-67, keeping `canSend`):

```tsx
  // An empty, unfocused bar stays quiet chrome — but a draft must remain
  // sendable after the user taps away to the terminal to hit a special key.
  const showSend = focused || text.length > 0;
```

- [ ] **Step 4: Make both actions ghost glyphs**

Replace the attach button's className (the `bg-[hsl(0_0%_16%)]` fill goes; `h-8 w-8` and `rounded-full` stay — they are the tap target and the focus ring's shape, not decoration):

```tsx
            className="focus-visible:ring-ring flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-[hsl(0_0%_60%)] outline-none focus-visible:ring-2"
```

Unwrap the send button from `{showSend && (...)}` so it renders unconditionally, and replace its className and comment:

```tsx
          <button
            type="button"
            aria-label="Send"
            disabled={!canSend}
            onMouseDown={(e) => e.preventDefault()}
            onClick={handleSend}
            // Always mounted, as in Slack. Disabled is a dimmer glyph rather
            // than a hidden control: at ghost weight it is quiet enough at
            // rest that hiding it bought nothing, and a visible-but-dim
            // control reads as "present, not yet available".
            // 45% is 3.86:1 on this surface — under the 4.5:1 text bar the
            // placeholder must clear, but over the 3:1 bar for glyphs.
            className="text-primary disabled:text-[hsl(0_0%_45%)] focus-visible:ring-ring flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full outline-none focus-visible:ring-2"
          >
            <SendHorizontal className="h-4 w-4" />
          </button>
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd web && pnpm test src/components/Terminal/ComposeBar.test.tsx`
Expected: all PASS. Note the pre-existing test `"prevents default on mousedown for the send button"` still passes — it focuses and types first, so send was already present for it.

- [ ] **Step 6: Typecheck**

Run: `cd web && pnpm exec tsc -b`
Expected: silent. If it reports `focused` as unused, that is wrong — `focused` still drives the panel's border and must remain.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/Terminal
git commit -m "feat(web): make the compose actions ghost glyphs with a permanent send

Retires showSend. With the placeholder swap already gone, the card edge
carries focus by itself."
```

---

### Task 6: The card

The structural task, and the one that moves the spacer constant. Read the Global Constraints again before starting — the spacer/panel invariant is what the drift guard exists to protect, and compaction is exactly the edit that would break it silently.

**Files:**
- Modify: `web/src/components/Terminal/ComposeBar.tsx` (spacer, panel, focus callback)
- Modify: `web/src/components/Terminal/TerminalToolbar.tsx` (bottom half, `focused` prop)
- Modify: `web/src/components/Terminal/index.tsx` (lift `focused`)
- Test: `web/src/components/Terminal/ComposeBar.test.tsx:392-430` (drift guard), `web/src/components/Terminal/TerminalToolbar.test.tsx` (append)

**Interfaces:**
- Consumes: nothing from Tasks 4-5 beyond the file state they leave.
- Produces: `ComposeBar` prop `onFocusedChange?: (focused: boolean) => void`; `TerminalToolbar` prop `focused?: boolean`.

**Height accounting** — the target, for reference while implementing:

```
                          today   naive card   compact
mt-* gap above              8          8           6
panel border-t              1          1           1
panel py                   12         12           8
row (mirror py + 21px)     33         33          33
toolbar border-t            1          1           1
toolbar button             40         40          40
toolbar border-b            0          1           1
mb-* gap below              0          8           6
                          ----       ----        ----
                           95        104          96
```

- [ ] **Step 1: Write the failing tests**

In `web/src/components/Terminal/ComposeBar.test.tsx`, update the drift guard (lines 392-430). Change these three assertions and their comments — the rest of that test is unchanged:

```tsx
    // one row (line-height + the mirror's py-1.5) + the panel's py-1 + its
    // 1px border-t. The panel's padding is compacted to pay for the card's
    // four-sided margin; the MIRROR keeps py-1.5, which is why
    // --compose-max-h below is untouched.
    expect(spacer.style.height).toBe(
      "calc(var(--compose-row-h) + 1.25rem + 1px)",
    );
```

```tsx
    // The formula's other two terms are class-driven, so pin them too: the
    // 1.25rem is the mirror's py-1.5 (12px) PLUS the panel's py-1 (8px), and
    // the 1px is the panel's border-t. Without these, changing the panel's
    // padding leaves the spacer at its old value while the panel resting
    // height moves — silent drift that every assertion above still passes.
    expect(panel.className.split(" ")).toContain("py-1");
    expect(panel.className.split(" ")).toContain("border-t");
    expect(mirror.className.split(" ")).toContain("py-1.5");
```

Then append two new tests to the `describe("ComposeBar overlay height")` block:

```tsx
  it("renders the panel as the card's top half", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const panel = screen.getByTestId("compose-grow-wrapper").parentElement!;
    const spacer = panel.parentElement!;

    // The panel and the toolbar are separate boxes that meet flush; each
    // renders half the card's border.
    expect(panel.className).toContain("rounded-t-lg");
    expect(panel.className).toContain("border-x");
    expect(panel.className).toContain("inset-x-2");
    expect(panel.className).toContain("border-[hsl(0_0%_20%)]");
    expect(spacer.className).toContain("mt-1.5");
  });

  it("reports focus changes so the toolbar half can brighten with the panel", () => {
    const onFocusedChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onFocusedChange,
    });

    fireEvent.focus(textarea());
    expect(onFocusedChange).toHaveBeenLastCalledWith(true);

    fireEvent.blur(textarea());
    expect(onFocusedChange).toHaveBeenLastCalledWith(false);
  });
```

In `web/src/components/Terminal/TerminalToolbar.test.tsx`, append inside the existing `describe("TerminalToolbar")`:

```tsx
  it("renders the card's bottom half", () => {
    render(<TerminalToolbar onKeyPress={() => {}} />);

    const bar = screen.getByTestId("terminal-toolbar");
    expect(bar.className).toContain("rounded-b-lg");
    expect(bar.className).toContain("border-x");
    expect(bar.className).toContain("border-b");
    expect(bar.className).toContain("mx-2");
    expect(bar.className).toContain("mb-1.5");
  });

  it("brightens its card edge with the compose bar, so the halves move together", () => {
    const { rerender } = render(
      <TerminalToolbar onKeyPress={() => {}} focused={false} />,
    );
    const bar = () => screen.getByTestId("terminal-toolbar");
    expect(bar().className).toContain("border-[hsl(0_0%_20%)]");

    rerender(<TerminalToolbar onKeyPress={() => {}} focused={true} />);
    expect(bar().className).toContain("border-[hsl(0_0%_30%)]");
  });

  it("keeps the row divider a distinct value from the card edge", () => {
    render(<TerminalToolbar onKeyPress={() => {}} focused={false} />);

    // The seam between the two halves is the divider, quieter than the card's
    // own edge so it separates without competing.
    //
    // This is also a twMerge guard. `border-t-<color>` and `border-<color>`
    // are conflicting groups: a generic `border-[...]` appearing AFTER
    // `border-t-[...]` in the cn() arguments silently deletes the divider.
    // The divider must therefore be the LAST border colour passed.
    const bar = screen.getByTestId("terminal-toolbar");
    expect(bar.className).toContain("border-t-[hsl(0_0%_14%)]");
  });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/components/Terminal/`
Expected: FAIL — spacer height is still `calc(var(--compose-row-h) + 1.5rem + 1px)`, the panel still has `py-1.5` and no `rounded-t-lg`, and `focused` is not a `TerminalToolbar` prop (TS error).

- [ ] **Step 3: Make the panel the card's top half**

In `web/src/components/Terminal/ComposeBar.tsx`, update the spacer. Change its className and the height expression (the long explanatory comment above it stays, but update its final formula line from `+ 0.75rem + 0.75rem + 1px` to `+ 0.75rem + 0.5rem + 1px`):

```tsx
      <div
        // mt-1.5 is the card's top margin. The 2026-08-04 refresh used 8px
        // here to turn an accidental sub-row remainder into a deliberate gap;
        // a bordered card states that boundary outright, so the gap no longer
        // has to carry the separation alone and pays 2px toward the card's
        // own margins. Margin, not padding: FitAddon measures the terminal
        // container, and padding would be counted as usable row space.
        className="relative mt-1.5 flex-shrink-0"
        style={
          {
            "--compose-row-h": "21px",
            height: "calc(var(--compose-row-h) + 1.25rem + 1px)",
          } as CSSProperties
        }
      >
```

Then the panel:

```tsx
        <div
          ref={panelRef}
          className={cn(
            // The card's top half. The panel is absolutely positioned and the
            // toolbar is in flow, so they cannot share one border box — each
            // renders half, and because the panel sits exactly on the spacer's
            // bottom edge while the toolbar begins immediately after it, the
            // two meet flush and the seam becomes the row divider.
            //
            // py-1 rather than py-1.5: the panel's padding is compacted to pay
            // for the card's four-sided margin. This is the PANEL's padding,
            // not the mirror's — the draft text keeps its own 6px inside the
            // card, which is why --compose-max-h is unchanged. The spacer
            // height above moves with this; they must not drift.
            "absolute inset-x-2 bottom-0 flex items-end gap-1.5 rounded-t-lg border-x border-t bg-input px-2 py-1 transition-colors",
            // Only the border animates, so the zone never changes value
            // underfoot. TerminalToolbar carries the same two values on the
            // bottom half via its `focused` prop.
            focused ? "border-[hsl(0_0%_30%)]" : "border-[hsl(0_0%_20%)]",
          )}
        >
```

- [ ] **Step 4: Report focus upward**

Add to `ComposeBarProps`:

```tsx
  /**
   * Focus is signalled by the whole card's edge, but the card's bottom half
   * is TerminalToolbar — a sibling component. This lifts the state so both
   * halves brighten together.
   */
  onFocusedChange?: (focused: boolean) => void;
```

Destructure `onFocusedChange`, add a handler next to the other callbacks:

```tsx
  const handleFocusChange = useCallback(
    (next: boolean) => {
      setFocused(next);
      onFocusedChange?.(next);
    },
    [onFocusedChange],
  );
```

and use it on the textarea:

```tsx
              onFocus={() => handleFocusChange(true)}
              onBlur={() => handleFocusChange(false)}
```

- [ ] **Step 5: Make the toolbar the card's bottom half**

In `web/src/components/Terminal/TerminalToolbar.tsx`, add to `TerminalToolbarProps`:

```tsx
interface TerminalToolbarProps {
  onKeyPress: (key: string) => void;
  /** Mirrors ComposeBar's focus so both halves of the card brighten together. */
  focused?: boolean;
}
```

Destructure `focused = false` in the component signature, and replace the toolbar div's className:

```tsx
        className={cn(
          // The card's bottom half — see the matching comment on ComposeBar's
          // panel for why the card is two boxes rather than one.
          "scrollbar-none bg-input flex items-center min-[500px]:justify-center overflow-x-auto rounded-b-lg border-x border-t border-b mx-2 mb-1.5",
          focused ? "border-[hsl(0_0%_30%)]" : "border-[hsl(0_0%_20%)]",
          // The seam where the two halves meet is the row divider, at --border
          // (14%) — quieter than the card's own edge so it separates without
          // competing. It stays a border rather than becoming a child element
          // so the row height is unchanged and the divider costs nothing.
          //
          // MUST come last. twMerge treats `border-t-<color>` and
          // `border-<color>` as conflicting groups, so a generic border colour
          // appearing after this line would silently delete the divider.
          "border-t-[hsl(0_0%_14%)]",
        )}
```

- [ ] **Step 6: Lift the focus state in Terminal**

In `web/src/components/Terminal/index.tsx`, add state next to `composeOverlay`:

```tsx
    // The card's edge spans two sibling components, so its focus state lives
    // here rather than inside either half.
    const [composeFocused, setComposeFocused] = useState(false);
```

Extend the existing reset effect so a stale focused edge can't survive the bar being hidden:

```tsx
    useEffect(() => {
      if (!composeBarVisible) {
        setComposeOverlay(0);
        setComposeFocused(false);
      }
    }, [composeBarVisible]);
```

Wire both components:

```tsx
            <ComposeBar
              onSend={handleSend}
              connected={isConnected}
              workingDirectory={workingDirectory}
              sessionSlug={sessionSlug}
              onOverlayHeightChange={setComposeOverlay}
              onFocusedChange={setComposeFocused}
            />
            <TerminalToolbar onKeyPress={sendInput} focused={composeFocused} />
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd web && pnpm test src/components/Terminal/`
Expected: all PASS, including the updated drift guard and the five new card tests.

- [ ] **Step 8: Typecheck and run the full suite**

Run: `cd web && pnpm exec tsc -b`
Expected: silent.

Run: `cd web && pnpm test`
Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add web/src/components/Terminal
git commit -m "feat(web): rebuild the compose zone as a rounded inset card

Two flush halves rather than one box, since the panel is absolutely
positioned and the toolbar is in flow. Compacted to +1px of chrome by
tightening the panel's padding and the surrounding gaps."
```

---

### Task 7: Browser verification and spec reconciliation

jsdom has no layout engine, so every pixel claim in this plan is so far unverified. This task measures them in a real browser and reconciles the spec with what actually shipped — the 2026-08-04 spec has the precedent for this ("Amended after implementation").

**Files:**
- Modify: `docs/superpowers/specs/2026-08-05-slack-compose-card-design.md` (amendments, if measurements differ)

**Interfaces:**
- Consumes: the completed Tasks 1-6.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Start the dev server**

The SPA is embedded in the argus binary, so editing web sources does not change what the tailnet URL serves — verify against Vite, not the installed argus.

Run: `cd web && pnpm dev`
Expected: serving on `http://localhost:5273`, proxying API and WebSocket traffic to `http://localhost:3000` (the running argus node).

- [ ] **Step 2: Emulate a phone and open a session**

Using the chrome-devtools tools: resize the page to 390×844, emulate `pointer: coarse` so `useViewport` reports mobile, navigate to `http://localhost:5273`, and open any session with a live terminal.

Expected: the compose card is visible as a single rounded rectangle inset from all four screen edges, with the key row inside its bottom.

- [ ] **Step 3: Measure the resting geometry**

Evaluate in the page:

```js
const spacer = document.querySelector('[data-testid="compose-grow-wrapper"]')
  .parentElement.parentElement;
const panel = spacer.firstElementChild;
const toolbar = document.querySelector('[data-testid="terminal-toolbar"]');
JSON.stringify({
  spacerH: spacer.getBoundingClientRect().height,
  panelH: panel.getBoundingClientRect().height,
  toolbarH: toolbar.getBoundingClientRect().height,
  zoneTop: spacer.getBoundingClientRect().top,
  viewportH: window.innerHeight,
  seamGap:
    toolbar.getBoundingClientRect().top - panel.getBoundingClientRect().bottom,
}, null, 2);
```

Expected: `spacerH === panelH === 42` (the invariant — if these differ, stop and fix before going further), `toolbarH === 42`, and `seamGap === 0`. Total zone including margins should measure 96px against the pre-change 95px.

- [ ] **Step 4: Confirm growth still never refits the terminal**

Type three lines into the compose input, then re-run the measurement from Step 3 plus:

```js
const ta = document.querySelector('[data-testid="compose-grow-wrapper"] textarea');
JSON.stringify({
  panelH: ta.closest('.absolute').getBoundingClientRect().height,
  taScrollH: ta.scrollHeight,
  taClientH: ta.clientHeight,
  terminalRows: document.querySelectorAll('.xterm-rows > div').length,
}, null, 2);
```

Expected: the panel caps at 42 + 2 rows = 84px; past three lines `taScrollH > taClientH` (scrolling, not clipping); `terminalRows` is unchanged from its resting value throughout.

- [ ] **Step 5: Inspect the two visual risks the unit tests cannot cover**

Take a screenshot at rest and at full growth. Check:

1. **The seam.** Where the two halves meet there must be exactly one hairline — no subpixel gap and no doubled 2px line. `seamGap` from Step 3 catches the gap; the screenshot catches the doubling.
2. **The corners.** Scroll the key row to each end. The first and last keys must not visually collide with the card's rounded bottom corners.

If either fails, the fix is a spec amendment plus a follow-up commit — record what you changed.

- [ ] **Step 6: Confirm the placeholder against a real session**

Read back the placeholder and confirm it names the open session:

```js
document.querySelector('[data-testid="compose-grow-wrapper"] textarea').placeholder;
```

Expected: `Message #<slug>` matching that session's slug from `argus session ls`, truncated at 28 slug characters if longer.

- [ ] **Step 7: Reconcile the spec**

If any measurement differs from the spec's numbers, amend `docs/superpowers/specs/2026-08-05-slack-compose-card-design.md` in place, marking the change the way the parent spec does:

```markdown
**Amended after implementation.** <what actually shipped, and why>
```

If everything matched, add a one-line note under "Geometry" recording that the accounting was verified at 390×844, so a later reader knows the numbers are measured rather than derived.

- [ ] **Step 8: Commit**

```bash
git add -f docs/superpowers/specs/2026-08-05-slack-compose-card-design.md
git commit -m "docs: reconcile the compose card spec with measured geometry"
```

Note: `docs/superpowers/` is gitignored but tracked, so `git add` needs `-f` and must be run as its own command — chaining `git add && git commit` fails on the non-zero exit.

---

## Notes for the implementer

**Why the card is two boxes.** The obvious implementation — one wrapper div with a border around both rows — cannot work. The compose panel is `absolute` and anchored to the spacer's bottom edge so it can grow upward without changing the terminal's laid-out height; the toolbar is a normal flow sibling. A shared border box would force the toolbar into the absolutely-positioned subtree, which reintroduces exactly the refit-on-growth problem the 2026-08-04 architecture exists to prevent.

**Why the mirror keeps `py-1.5` while the panel goes to `py-1`.** They are different elements at different levels. The mirror's padding is part of the row height and feeds `--compose-max-h`; the panel's padding sits outside the row and feeds only the spacer height. Compacting the mirror instead would have changed the three-line cap and cramped the draft text against the card edge.

**The twMerge ordering trap in Task 6.** `cn` is `twMerge(clsx(...))`. twMerge knows `border-<color>` conflicts with `border-t-<color>` and keeps the last one. The toolbar passes both — a generic card colour for three sides and a specific divider colour for the top — so the divider must be the final argument. Reordering those two lines for tidiness silently deletes the row divider, and no assertion except the dedicated test in Task 6 Step 1 would notice.
