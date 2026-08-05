# Mobile Persistent Compose Input Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the modal compose overlay in mobile terminal view with a permanent one-line input that grows to three lines as an overlay, so the terminal is never resized while typing.

**Architecture:** A new `ComposeBar` component sits in flow between the xterm container and `TerminalToolbar`, occupying a fixed one-line-tall spacer. Its actual panel is absolutely positioned against the bottom of that spacer and grows *upward* via a CSS grid auto-grow trick, so the terminal's laid-out height never changes and `FitAddon` never refits. `ComposeBar` reports its overlay height through a `ResizeObserver`, and `Terminal` shifts the xterm container up by that amount with a CSS `transform` — layout-neutral, so still no refit. Separately, sending text now exits tmux copy-mode server-side (attached sessions) and scrolls xterm to the bottom client-side (unattached tabs).

**Tech Stack:** React 19, TypeScript, Tailwind v4, vitest + @testing-library/react (jsdom), Go 1.x with `os/exec` for tmux.

## Global Constraints

- Mobile only. Desktop must be untouched — `TerminalToolbar` and the compose UI already render only under `isMobile` (`web/src/components/Terminal/index.tsx:184`).
- No changes to the WebSocket protocol or to `injectCompose`'s bracketed-paste semantics.
- No draft persistence across page reloads.
- Frontend test command: `pnpm test` from `web/`. Single file: `pnpm vitest run <path>`.
- Go test command: `go test ./internal/node/terminal/` from the repo root.
- tmux integration tests must never touch the default tmux socket — production argus runs on this box with live sessions. The tasks here add no tmux integration tests, only unit tests with injected fakes.
- UI verification happens on the vite dev server (`pnpm dev`, port **5273**), never against the installed argus binary — the SPA is embedded in the binary and would need a restart to reflect changes.
- `docs/superpowers/` is covered by a global gitignore. Committing files there requires `git add -f`, and `git add` must be run as its own command, never chained with `&&` before `git commit`.

---

### Task 1: Exit tmux copy-mode before injecting compose text

Independent of the UI work — this bug already affects today's compose modal. Landing it first means it ships even if the UI work stalls.

**Background for the implementer:** The web terminal is a real `tmux attach-session` (`internal/node/terminal/handler.go:368`). `injectCompose` writes to `s.ptmx`, which is the PTY of the tmux *client*, so those bytes arrive as client keystrokes. When the pane is in copy-mode (which mobile touch-scroll triggers, since the seeded tmux config sets `mouse on`), tmux routes them through the copy-mode key table instead of to the pane's process. The message never reaches the agent and its characters fire copy-mode bindings on the way past. Cancelling any pane mode first fixes it, and has the desirable side effect of snapping the pane back to live output.

**Files:**
- Modify: `internal/node/terminal/handler.go` (the `"text"` case at `handler.go:141`, and `handleConnection` at `handler.go:281`)
- Test: `internal/node/terminal/handler_test.go`

**Interfaces:**
- Consumes: existing `injectCompose(w io.Writer, sleep func(time.Duration), text string, submit bool) error`
- Produces: `newExitPaneMode(sessionName string) func() error` (returns `nil` for an empty name) and `handleTextMessage(w io.Writer, sleep func(time.Duration), exitMode func() error, text string, submit bool) error`

- [ ] **Step 1: Write the failing tests**

Append to `internal/node/terminal/handler_test.go`:

```go
// orderRecorder records exitPaneMode calls and PTY writes in one ordered log so
// tests can assert the copy-mode cancel happens BEFORE the paste block.
type orderRecorder struct{ events []string }

func (r *orderRecorder) Write(p []byte) (int, error) {
	r.events = append(r.events, "write:"+string(p))
	return len(p), nil
}

func TestHandleTextMessageCancelsPaneModeBeforePaste(t *testing.T) {
	r := &orderRecorder{}
	exitMode := func() error {
		r.events = append(r.events, "exit")
		return nil
	}

	if err := handleTextMessage(r, func(time.Duration) {}, exitMode, "hello", true); err != nil {
		t.Fatalf("handleTextMessage: %v", err)
	}

	if len(r.events) != 3 {
		t.Fatalf("want 3 events (exit, paste, return), got %d: %v", len(r.events), r.events)
	}
	if r.events[0] != "exit" {
		t.Fatalf("copy-mode cancel must come first, got %q", r.events[0])
	}
	if r.events[1] != "write:\x1b[200~hello\x1b[201~" {
		t.Fatalf("second event must be the paste block, got %q", r.events[1])
	}
	if r.events[2] != "write:\r" {
		t.Fatalf("third event must be the submit Return, got %q", r.events[2])
	}
}

func TestHandleTextMessageIgnoresExitPaneModeError(t *testing.T) {
	r := &orderRecorder{}
	exitMode := func() error { return errors.New("no current mode") }

	if err := handleTextMessage(r, func(time.Duration) {}, exitMode, "hello", false); err != nil {
		t.Fatalf("a failing cancel must not fail the send: %v", err)
	}
	if len(r.events) != 1 || r.events[0] != "write:\x1b[200~hello\x1b[201~" {
		t.Fatalf("paste must still be written, got %v", r.events)
	}
}

func TestHandleTextMessageSkipsNilExitMode(t *testing.T) {
	r := &orderRecorder{}

	if err := handleTextMessage(r, func(time.Duration) {}, nil, "hello", false); err != nil {
		t.Fatalf("handleTextMessage: %v", err)
	}
	if len(r.events) != 1 || r.events[0] != "write:\x1b[200~hello\x1b[201~" {
		t.Fatalf("nil exitMode must write only the paste block, got %v", r.events)
	}
}

func TestNewExitPaneModeNilForRawShellRoute(t *testing.T) {
	// The raw-shell route (/ws/terminal) has no tmux session, so there is no
	// pane to cancel.
	if newExitPaneMode("") != nil {
		t.Fatal("empty session name must yield a nil exitMode")
	}
	if newExitPaneMode("argus-abc123") == nil {
		t.Fatal("named session must yield a non-nil exitMode")
	}
}
```

Add `"errors"` to the test file's import block if it is not already there.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/node/terminal/ -run 'TestHandleTextMessage|TestNewExitPaneMode' -v`

Expected: FAIL to compile — `undefined: handleTextMessage`, `undefined: newExitPaneMode`.

- [ ] **Step 3: Implement**

In `internal/node/terminal/handler.go`, add to the `session` struct (`handler.go:27`):

```go
type session struct {
	ws        *websocket.Conn
	ptmx      *os.File
	cmd       *exec.Cmd
	wsMu      sync.Mutex
	done      chan struct{}
	closeOnce sync.Once
	// exitPaneMode cancels any tmux pane mode (copy-mode, choose-tree, ...)
	// before compose text is injected. nil for the raw-shell route, which has
	// no tmux session.
	exitPaneMode func() error
}
```

Add below `injectCompose`:

```go
// exitPaneModeTimeout bounds the tmux call so a wedged tmux cannot stall the
// WebSocket read loop, which is what calls this.
const exitPaneModeTimeout = 2 * time.Second

// newExitPaneMode returns a func that cancels any mode the session's active
// pane is in, or nil when there is no tmux session (the raw-shell route).
//
// This matters because the PTY we write compose text to is a tmux *client*: a
// pane sitting in copy-mode routes those bytes through the copy-mode key table
// instead of to the pane's process, so the paste never reaches the agent and
// its characters fire copy-mode bindings instead ('q' cancels, '/' opens
// search, digits become repeat counts). Cancelling first also snaps the pane
// back to live output, which is the feedback the user wants after sending.
func newExitPaneMode(sessionName string) func() error {
	if sessionName == "" {
		return nil
	}
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), exitPaneModeTimeout)
		defer cancel()
		cmd, err := shared.TmuxCommandContext(ctx, "send-keys", "-t", sessionName, "-X", "cancel")
		if err != nil {
			return err
		}
		// Errors here are expected and benign: tmux fails this when the pane is
		// not in a mode, which is the common case.
		return cmd.Run()
	}
}

// handleTextMessage injects a compose "text" message, cancelling any tmux pane
// mode first. A failing cancel never blocks the send — see newExitPaneMode.
func handleTextMessage(w io.Writer, sleep func(time.Duration), exitMode func() error, text string, submit bool) error {
	if exitMode != nil {
		_ = exitMode()
	}
	return injectCompose(w, sleep, text, submit)
}
```

Ensure `"context"` is in the import block.

Replace the `"text"` case body (`handler.go:141-149`) with:

```go
			case "text":
				if msg.Text != "" {
					// Submit defaults to true for backwards compatibility
					// (old clients don't send the field).
					submit := msg.Submit == nil || *msg.Submit
					if err := handleTextMessage(s.ptmx, time.Sleep, s.exitPaneMode, msg.Text, submit); err != nil {
						return
					}
				}
```

In `handleConnection`, populate the field where the session is constructed (`handler.go:312`):

```go
	s := &session{
		ws:           ws,
		ptmx:         ptmx,
		cmd:          cmd,
		done:         make(chan struct{}),
		exitPaneMode: newExitPaneMode(sessionName),
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/node/terminal/ -v`

Expected: PASS, including the pre-existing `TestInjectCompose*` and `TestComposePasteBlock*` tests.

- [ ] **Step 5: Verify the whole build still compiles**

Run: `go build ./...`

Expected: no output, exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/node/terminal/handler.go internal/node/terminal/handler_test.go
git commit -m "fix(terminal): exit tmux copy-mode before injecting compose text

The PTY we write compose text to is a tmux client, so a pane sitting in
copy-mode routes those bytes through the copy-mode key table instead of to
the pane's process. The message never reaches the agent and its characters
fire copy-mode bindings on the way past. Cancel any pane mode first."
```

---

### Task 2: `insertPaths` helper

Pure extraction of the cursor-insertion logic currently inlined in `ComposeInput.handleFilesPicked` (`web/src/components/Terminal/TerminalToolbar.tsx:96-127`). Pulling it out makes the rule set testable without any DOM layout.

**Files:**
- Create: `web/src/components/Terminal/insertPaths.ts`
- Test: `web/src/components/Terminal/insertPaths.test.ts`

**Interfaces:**
- Consumes: `shellEscape` from `web/src/lib/shell.ts`
- Produces: `insertPaths(text: string, selectionStart: number, selectionEnd: number, paths: string[]): { text: string; cursor: number }`

- [ ] **Step 1: Write the failing test**

Create `web/src/components/Terminal/insertPaths.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import { insertPaths } from "./insertPaths";

describe("insertPaths", () => {
  it("inserts into empty text with no padding spaces", () => {
    expect(insertPaths("", 0, 0, ["/tmp/a.txt"])).toEqual({
      text: "/tmp/a.txt",
      cursor: 10,
    });
  });

  it("joins multiple paths with a single space", () => {
    expect(insertPaths("", 0, 0, ["/tmp/a", "/tmp/b"])).toEqual({
      text: "/tmp/a /tmp/b",
      cursor: 13,
    });
  });

  it("shell-escapes paths containing spaces", () => {
    const result = insertPaths("", 0, 0, ["/tmp/my file"]);
    expect(result.text).toBe("'/tmp/my file'");
    expect(result.cursor).toBe(14);
  });

  it("adds a leading space when the text before the cursor does not end in one", () => {
    expect(insertPaths("look at", 7, 7, ["/tmp/a"])).toEqual({
      text: "look at /tmp/a",
      cursor: 14,
    });
  });

  it("does not double the leading space when one is already there", () => {
    expect(insertPaths("look at ", 8, 8, ["/tmp/a"])).toEqual({
      text: "look at /tmp/a",
      cursor: 14,
    });
  });

  it("adds a trailing space when the text after the cursor does not start with one", () => {
    expect(insertPaths("ab", 1, 1, ["/tmp/a"])).toEqual({
      text: "a /tmp/a b",
      cursor: 8,
    });
  });

  it("replaces the current selection", () => {
    expect(insertPaths("keep DROP keep", 5, 9, ["/tmp/a"])).toEqual({
      text: "keep /tmp/a keep",
      cursor: 11,
    });
  });

  it("returns the text unchanged when given no paths", () => {
    expect(insertPaths("hello", 5, 5, [])).toEqual({
      text: "hello",
      cursor: 5,
    });
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm vitest run src/components/Terminal/insertPaths.test.ts`

Expected: FAIL — `Failed to resolve import "./insertPaths"`.

- [ ] **Step 3: Implement**

Create `web/src/components/Terminal/insertPaths.ts`:

```ts
import { shellEscape } from "@/lib/shell";

export interface InsertPathsResult {
  /** The full new textarea value. */
  text: string;
  /** Where the caret should sit afterwards — just past the inserted paths. */
  cursor: number;
}

/**
 * Insert shell-escaped paths into `text`, replacing the range
 * [selectionStart, selectionEnd). Separator spaces are added only where the
 * adjacent text needs them, so the result never has doubled spaces.
 */
export function insertPaths(
  text: string,
  selectionStart: number,
  selectionEnd: number,
  paths: string[],
): InsertPathsResult {
  if (paths.length === 0) return { text, cursor: selectionEnd };

  const insert = paths.map(shellEscape).join(" ");
  const before = text.slice(0, selectionStart);
  const after = text.slice(selectionEnd);

  const needsLeadingSpace = before.length > 0 && !before.endsWith(" ");
  const needsTrailingSpace = after.length > 0 && !after.startsWith(" ");

  return {
    text:
      before +
      (needsLeadingSpace ? " " : "") +
      insert +
      (needsTrailingSpace ? " " : "") +
      after,
    cursor: before.length + (needsLeadingSpace ? 1 : 0) + insert.length,
  };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm vitest run src/components/Terminal/insertPaths.test.ts`

Expected: PASS, 8 tests.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Terminal/insertPaths.ts web/src/components/Terminal/insertPaths.test.ts
git commit -m "refactor(terminal): extract insertPaths from the compose modal"
```

---

### Task 3: `ComposeBar` — send, disabled states, focus affordance

Builds the component and everything about it that jsdom can observe. Growth measurement is Task 4.

**Note on one refinement to the spec:** the spec's affordance table says the send button is "hidden" when the terminal has focus. Taken literally, a user with a two-line draft who taps the terminal to hit `esc` could no longer send without refocusing. The rule implemented here is **visible when the input is focused OR the draft is non-empty** — which preserves the spec's intent (an empty, unfocused bar is quiet chrome) without stranding a draft.

**Files:**
- Create: `web/src/components/Terminal/ComposeBar.tsx`
- Test: `web/src/components/Terminal/ComposeBar.test.tsx`

**Interfaces:**
- Consumes: `insertPaths` (Task 2); `FilePicker` from `@/components/FilePicker` (props: `open`, `onOpenChange`, `onPick`, `searchPath`); `cn` from `@/lib/utils`
- Produces: `ComposeBar` with props
  ```ts
  interface ComposeBarProps {
    onSend: (text: string) => void;
    connected: boolean;
    workingDirectory?: string | null;
    onOverlayHeightChange?: (height: number) => void;
  }
  ```
  `onOverlayHeightChange` is wired in Task 4; it is declared here so the prop type does not change between tasks.

- [ ] **Step 1: Write the failing test**

Create `web/src/components/Terminal/ComposeBar.test.tsx`:

```tsx
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ComposeBar } from "./ComposeBar";

afterEach(cleanup);

function textarea() {
  return screen.getByRole("textbox") as HTMLTextAreaElement;
}

describe("ComposeBar", () => {
  it("sends the typed text and clears the input", () => {
    const onSend = vi.fn();
    render(<ComposeBar onSend={onSend} connected />);

    fireEvent.change(textarea(), { target: { value: "run the tests" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(onSend).toHaveBeenCalledWith("run the tests");
    expect(textarea().value).toBe("");
  });

  it("keeps focus on the input after sending so the keyboard stays up", () => {
    render(<ComposeBar onSend={() => {}} connected />);

    fireEvent.focus(textarea());
    fireEvent.change(textarea(), { target: { value: "hi" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(document.activeElement).toBe(textarea());
  });

  it("disables send when the draft is only whitespace", () => {
    render(<ComposeBar onSend={() => {}} connected />);

    fireEvent.change(textarea(), { target: { value: "   " } });

    expect(screen.getByRole("button", { name: /send/i })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("disables send while disconnected but preserves the draft", () => {
    render(<ComposeBar onSend={() => {}} connected={false} />);

    fireEvent.change(textarea(), { target: { value: "queued message" } });

    expect(screen.getByRole("button", { name: /send/i })).toHaveProperty(
      "disabled",
      true,
    );
    expect(textarea().value).toBe("queued message");
  });

  it("hides the send button only when the bar is unfocused AND empty", () => {
    render(<ComposeBar onSend={() => {}} connected />);

    expect(screen.queryByRole("button", { name: /send/i })).toBeNull();

    fireEvent.focus(textarea());
    expect(screen.queryByRole("button", { name: /send/i })).not.toBeNull();

    fireEvent.change(textarea(), { target: { value: "draft" } });
    fireEvent.blur(textarea());
    // A draft must stay sendable after tapping away to the terminal.
    expect(screen.queryByRole("button", { name: /send/i })).not.toBeNull();
  });

  it("swaps the placeholder between focused and unfocused", () => {
    render(<ComposeBar onSend={() => {}} connected />);

    expect(textarea().placeholder).toBe("Tap to compose");

    fireEvent.focus(textarea());
    expect(textarea().placeholder).toBe("Message…");

    fireEvent.blur(textarea());
    expect(textarea().placeholder).toBe("Tap to compose");
  });

  it("never dims the draft text itself when focus leaves", () => {
    render(<ComposeBar onSend={() => {}} connected />);

    fireEvent.change(textarea(), { target: { value: "still readable" } });
    fireEvent.blur(textarea());

    // Dimming is chrome-only — the textarea must not carry an opacity class.
    expect(textarea().className).not.toMatch(/opacity-/);
  });

  it("does not send on Enter — Enter inserts a newline", () => {
    const onSend = vi.fn();
    render(<ComposeBar onSend={onSend} connected />);

    fireEvent.change(textarea(), { target: { value: "line one" } });
    fireEvent.keyDown(textarea(), { key: "Enter" });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("opens the file picker from the attach button", () => {
    render(<ComposeBar onSend={() => {}} connected workingDirectory="/w" />);

    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /attach/i }));

    expect(screen.queryByRole("dialog")).not.toBeNull();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm vitest run src/components/Terminal/ComposeBar.test.tsx`

Expected: FAIL — `Failed to resolve import "./ComposeBar"`.

- [ ] **Step 3: Implement**

Create `web/src/components/Terminal/ComposeBar.tsx`:

```tsx
import { memo, useCallback, useRef, useState } from "react";
import { SendHorizontal, Paperclip } from "lucide-react";
import { cn } from "@/lib/utils";
import { FilePicker } from "@/components/FilePicker";
import { insertPaths } from "./insertPaths";

interface ComposeBarProps {
  onSend: (text: string) => void;
  /** Send is disabled while the socket is down; the draft is kept. */
  connected: boolean;
  /** Session working directory, used to anchor file search. */
  workingDirectory?: string | null;
  /**
   * Reports how far the panel currently overflows its one-line spacer, so the
   * terminal can be shifted up by the same amount. Wired in Task 4.
   */
  onOverlayHeightChange?: (height: number) => void;
}

export const ComposeBar = memo(function ComposeBar({
  onSend,
  connected,
  workingDirectory,
}: ComposeBarProps) {
  const [text, setText] = useState("");
  const [focused, setFocused] = useState(false);
  const [showFilePicker, setShowFilePicker] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const canSend = connected && text.trim().length > 0;
  // An empty, unfocused bar stays quiet chrome — but a draft must remain
  // sendable after the user taps away to the terminal to hit a special key.
  const showSend = focused || text.length > 0;

  const handleSend = useCallback(() => {
    if (!canSend) return;
    onSend(text);
    setText("");
    // Keep the keyboard up for the next message.
    textareaRef.current?.focus();
  }, [canSend, onSend, text]);

  const handleFilesPicked = useCallback(
    (paths: string[]) => {
      const ta = textareaRef.current;
      const start = ta?.selectionStart ?? text.length;
      const end = ta?.selectionEnd ?? text.length;
      const result = insertPaths(text, start, end, paths);
      setText(result.text);
      requestAnimationFrame(() => {
        ta?.focus();
        ta?.setSelectionRange(result.cursor, result.cursor);
      });
    },
    [text],
  );

  return (
    <>
      {/* Spacer: holds the one-line height in flow. The panel below is
          absolutely positioned against its bottom edge and grows UPWARD, so
          growing the input never changes the terminal's laid-out height and
          never triggers a FitAddon refit. */}
      <div className="relative h-11 flex-shrink-0">
        <div
          className={cn(
            "absolute inset-x-0 bottom-0 flex items-end gap-1.5 border-t px-2 py-1.5 backdrop-blur transition-colors",
            focused
              ? "border-white/25 bg-secondary/50"
              : "border-white/10 bg-transparent",
          )}
        >
          <button
            type="button"
            aria-label="Attach files"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setShowFilePicker(true)}
            className="text-secondary-foreground flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-white/20"
          >
            <Paperclip className="h-4 w-4" />
          </button>

          <textarea
            ref={textareaRef}
            rows={1}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            placeholder={focused ? "Message…" : "Tap to compose"}
            className="min-h-8 w-full flex-1 resize-none bg-transparent py-1.5 text-sm focus:outline-none"
          />

          {showSend && (
            <button
              type="button"
              aria-label="Send"
              disabled={!canSend}
              onMouseDown={(e) => e.preventDefault()}
              onClick={handleSend}
              className="text-secondary-foreground flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-white/20 disabled:opacity-30"
            >
              <SendHorizontal className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      <FilePicker
        open={showFilePicker}
        onOpenChange={setShowFilePicker}
        onPick={handleFilesPicked}
        searchPath={workingDirectory ?? undefined}
      />
    </>
  );
});
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm vitest run src/components/Terminal/ComposeBar.test.tsx`

Expected: PASS, 9 tests.

If the file-picker test fails because `FilePicker` does not expose `role="dialog"`, open `web/src/components/FilePicker/index.tsx`, find the actual root element it renders when `open` is true, and assert on that instead — do not add a role to `FilePicker` just to satisfy the test.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Terminal/ComposeBar.tsx web/src/components/Terminal/ComposeBar.test.tsx
git commit -m "feat(terminal): add persistent mobile ComposeBar"
```

---

### Task 4: Auto-grow to three lines and report the overlay height

**Why the growth has no JS measurement:** jsdom performs no layout, so `scrollHeight` and `offsetHeight` are always `0` — the usual "set height to scrollHeight" auto-grow cannot be unit-tested at all. This uses the CSS grid replicated-content trick instead: an invisible mirror `div` holding the same text sits in the same grid cell as the textarea, so the grid row sizes itself to the content with no JavaScript. That leaves only one piece of real logic to test — converting an observed panel height into an overlay height — which a stubbed `ResizeObserver` can drive deterministically.

The trailing `" "` appended to the mirror's content is what makes a trailing newline reserve a line; without it the bar does not grow until the next character is typed.

**Files:**
- Modify: `web/src/components/Terminal/ComposeBar.tsx`
- Test: `web/src/components/Terminal/ComposeBar.test.tsx`

**Interfaces:**
- Consumes: `ComposeBarProps.onOverlayHeightChange` declared in Task 3
- Produces: `onOverlayHeightChange(height)` fires with `Math.max(0, observedHeight - collapsedHeight)`, where `collapsedHeight` is the first height observed after mount

- [ ] **Step 1: Write the failing test**

Add to `web/src/components/Terminal/ComposeBar.test.tsx`. Put the mock class and the `beforeEach` above the existing `describe` block, then append the new `describe`:

```tsx
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, act } from "@testing-library/react";
import { ComposeBar } from "./ComposeBar";

// jsdom has no ResizeObserver and no layout engine, so tests drive the observer
// by hand to assert the observed-height -> overlay-height conversion.
class MockResizeObserver {
  static instances: MockResizeObserver[] = [];
  callback: ResizeObserverCallback;

  constructor(cb: ResizeObserverCallback) {
    this.callback = cb;
    MockResizeObserver.instances.push(this);
  }
  observe() {}
  unobserve() {}
  disconnect() {}

  emit(height: number) {
    act(() => {
      this.callback(
        [{ contentRect: { height } } as ResizeObserverEntry],
        this as unknown as ResizeObserver,
      );
    });
  }
}

beforeEach(() => {
  MockResizeObserver.instances = [];
  globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver;
});

function observer() {
  const ro = MockResizeObserver.instances[0];
  if (!ro) throw new Error("ComposeBar did not observe its panel");
  return ro;
}
```

```tsx
describe("ComposeBar overlay height", () => {
  it("reports zero for the first observed height, which is the collapsed baseline", () => {
    const onOverlayHeightChange = vi.fn();
    render(
      <ComposeBar
        onSend={() => {}}
        connected
        onOverlayHeightChange={onOverlayHeightChange}
      />,
    );

    observer().emit(44);

    expect(onOverlayHeightChange).toHaveBeenCalledWith(0);
  });

  it("reports the overflow past the collapsed baseline as the panel grows", () => {
    const onOverlayHeightChange = vi.fn();
    render(
      <ComposeBar
        onSend={() => {}}
        connected
        onOverlayHeightChange={onOverlayHeightChange}
      />,
    );

    observer().emit(44); // baseline, one line
    observer().emit(64); // two lines
    observer().emit(84); // three lines

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(40);
  });

  it("never reports a negative height if the panel measures under its baseline", () => {
    const onOverlayHeightChange = vi.fn();
    render(
      <ComposeBar
        onSend={() => {}}
        connected
        onOverlayHeightChange={onOverlayHeightChange}
      />,
    );

    observer().emit(44);
    observer().emit(30);

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("returns to zero when the draft is cleared by sending", () => {
    const onOverlayHeightChange = vi.fn();
    render(
      <ComposeBar
        onSend={() => {}}
        connected
        onOverlayHeightChange={onOverlayHeightChange}
      />,
    );

    observer().emit(44);
    observer().emit(84);
    observer().emit(44);

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("mirrors the draft text so the grid row grows without measuring", () => {
    render(<ComposeBar onSend={() => {}} connected />);

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "line one\nline two" },
    });

    const mirror = document.querySelector("[data-testid='compose-mirror']");
    // The trailing space is what makes a trailing newline reserve a line.
    expect(mirror?.textContent).toBe("line one\nline two ");
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm vitest run src/components/Terminal/ComposeBar.test.tsx`

Expected: the 5 new tests FAIL — `ComposeBar did not observe its panel`, and the mirror query returns `null`. The 9 tests from Task 3 still PASS.

- [ ] **Step 3: Implement**

In `web/src/components/Terminal/ComposeBar.tsx`, add `useEffect` to the React import, then add this above the `return`:

```tsx
  const panelRef = useRef<HTMLDivElement>(null);
  // Height of the panel at one line, captured from the first observation. The
  // spacer's h-11 is only a pre-measurement fallback; everything downstream
  // uses the measured value, so no hardcoded pixel height can drift.
  const collapsedHeightRef = useRef<number | null>(null);

  useEffect(() => {
    const el = panelRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;

    const ro = new ResizeObserver((entries) => {
      const height = entries[0]?.contentRect.height ?? 0;
      if (collapsedHeightRef.current === null) {
        collapsedHeightRef.current = height;
      }
      onOverlayHeightChange?.(
        Math.max(0, height - (collapsedHeightRef.current ?? height)),
      );
    });

    ro.observe(el);
    return () => ro.disconnect();
  }, [onOverlayHeightChange]);
```

Add `onOverlayHeightChange` to the destructured props.

Attach `ref={panelRef}` to the absolutely positioned panel `div`, and replace the bare `<textarea>` with the grid auto-grow wrapper:

```tsx
          {/* CSS-only auto-grow: an invisible mirror in the same grid cell
              sizes the row to the content, so no JS measures anything. The
              wrapper caps at three lines and the textarea scrolls inside it. */}
          <div className="grid max-h-[3lh] flex-1 overflow-hidden">
            <div
              data-testid="compose-mirror"
              aria-hidden="true"
              className="invisible [grid-area:1/1/2/2] py-1.5 text-sm break-words whitespace-pre-wrap"
            >
              {text + " "}
            </div>
            <textarea
              ref={textareaRef}
              rows={1}
              value={text}
              onChange={(e) => setText(e.target.value)}
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              placeholder={focused ? "Message…" : "Tap to compose"}
              className="min-h-8 w-full resize-none overflow-y-auto bg-transparent py-1.5 text-sm [grid-area:1/1/2/2] focus:outline-none"
            />
          </div>
```

The mirror and the textarea must keep identical `py-1.5`, `text-sm`, and wrapping classes — if they drift, the row sizes to the wrong content and the bar grows early or late.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && pnpm vitest run src/components/Terminal/ComposeBar.test.tsx`

Expected: PASS, 14 tests.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Terminal/ComposeBar.tsx web/src/components/Terminal/ComposeBar.test.tsx
git commit -m "feat(terminal): grow ComposeBar to three lines and report overlay height"
```

---

### Task 5: Wire `ComposeBar` into `Terminal`

**Files:**
- Modify: `web/src/components/Terminal/index.tsx`

**Interfaces:**
- Consumes: `ComposeBar` (Tasks 3–4); `sendText` and `xtermRef` from `useTerminalConnection` (`web/src/components/Terminal/hooks/useTerminalConnection.ts:61`)
- Produces: nothing consumed by later tasks

**Why there is no component test here:** `Terminal` instantiates xterm, which requires canvas and WebGL, plus a live `WebSocket`. It cannot render in jsdom, and stubbing enough of it to mount would test the stubs. Verification is a type check plus the manual check in Step 4, which pins the one behaviour that actually matters (the PTY is never resized) to an observable fact rather than a rendering detail.

- [ ] **Step 1: Add the overlay-height state and the send handler**

In `web/src/components/Terminal/index.tsx`, add `useState` to the React import and `ComposeBar` to the local imports:

```tsx
import { ComposeBar } from "./ComposeBar";
```

Add inside the component, next to the other hooks:

```tsx
    // How far the compose panel currently overflows its one-line spacer. The
    // terminal is shifted up by this much with a transform rather than being
    // resized: transforms don't affect layout, so FitAddon never refits and the
    // PTY never sees a SIGWINCH mid-typing (which would repaint the agent's TUI
    // and reflow wrapped scrollback out from under the user).
    const [composeOverlay, setComposeOverlay] = useState(0);

    // Sending while scrolled up must bring the user back to the live output —
    // xterm deliberately holds its scroll position when new output arrives, so
    // the reply would otherwise land off-screen and the terminal would look
    // frozen. (Attached tmux sessions are additionally snapped back server-side
    // by the copy-mode cancel; this covers the raw-shell route, which has no
    // tmux at all.)
    const handleSend = useCallback(
      (text: string) => {
        sendText(text);
        xtermRef.current?.scrollToBottom();
      },
      [sendText, xtermRef]
    );
```

- [ ] **Step 2: Apply the transform and render the bar**

Replace the terminal container `div` (`index.tsx:153-162`) with:

```tsx
        <div
          ref={terminalRef}
          className={cn(
            "terminal-container min-h-0 w-full flex-1 overflow-hidden",
            selectMode && "ring-primary ring-2 ring-inset"
          )}
          style={{
            transform: composeOverlay ? `translateY(-${composeOverlay}px)` : undefined,
            transition: "transform 150ms ease-out",
          }}
          onClick={handleContainerClick}
          onTouchStart={selectMode ? (e) => e.stopPropagation() : undefined}
          onTouchEnd={selectMode ? (e) => e.stopPropagation() : undefined}
        />
```

Replace the mobile toolbar block (`index.tsx:183-192`) with:

```tsx
        {/* Mobile: persistent compose input plus the special-keys toolbar.
            Both stay mounted so there is no layout shift and ^C / esc are
            always one tap away. */}
        {isMobile && !selectMode && (
          <>
            <ComposeBar
              onSend={handleSend}
              connected={isConnected}
              workingDirectory={workingDirectory}
              onOverlayHeightChange={setComposeOverlay}
            />
            <TerminalToolbar onKeyPress={sendInput} />
          </>
        )}
```

`TerminalToolbar` still has its old prop signature at this point; the extra props are simply no longer passed. Task 6 removes them. Leave `Terminal`'s `onAttachments` prop and its doc comment in place for now — Task 6 removes it together with its call site, so the tree never has a dangling reference.

- [ ] **Step 3: Type-check and run the full suite**

Run: `cd web && pnpm exec tsc -b`

Expected: no output, exit 0. If it reports that `onAttachments` is declared but never read, leave it — Task 6 removes it.

Run: `cd web && pnpm test`

Expected: PASS, all existing suites plus the 14 `ComposeBar` tests and 8 `insertPaths` tests.

- [ ] **Step 4: Verify on the dev server that the PTY is never resized**

This is the check the unit tests cannot make. Run it before committing.

1. `cd web && pnpm dev` — serves on port **5273**. Do not test against the installed argus binary; it serves an embedded copy of the SPA and would need a restart.
2. Open `http://localhost:5273`, then Chrome DevTools → device toolbar → iPhone 14 Pro.
3. Attach a session, then in the pane run `stty size` and note the two numbers (rows, columns).
4. Tap the compose input and type enough to wrap it to three lines. Watch the terminal: content must shift up smoothly, the bottom row must stay visible above the panel, and the agent's TUI must not repaint or reflow.
5. Run `stty size` again by sending it from the compose bar. **The rows value must be identical to step 3.** If it changed, the terminal is being resized and the transform is not doing its job.
6. Keep typing past three lines — the input must scroll internally rather than grow.
7. Tap the terminal body: the keyboard must stay up, the input must dim to "Tap to compose", and any draft must remain fully legible.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Terminal/index.tsx
git commit -m "feat(terminal): mount the persistent ComposeBar in mobile view

The compose panel grows upward over the terminal and the terminal is shifted
by a transform instead of being resized, so FitAddon never refits and the PTY
never sees a SIGWINCH while the user is typing."
```

---

### Task 6: Remove the compose modal, the two toolbar buttons, and the dead props

**Files:**
- Modify: `web/src/components/Terminal/TerminalToolbar.tsx`
- Modify: `web/src/components/Terminal/index.tsx`
- Modify: `web/src/components/Workspace/index.tsx:261`
- Test: `web/src/components/Terminal/TerminalToolbar.test.tsx`

**Interfaces:**
- Produces: `TerminalToolbar` with a single prop, `{ onKeyPress: (key: string) => void }`

- [ ] **Step 1: Write the failing test**

Create `web/src/components/Terminal/TerminalToolbar.test.tsx`:

```tsx
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { TerminalToolbar } from "./TerminalToolbar";

afterEach(cleanup);

describe("TerminalToolbar", () => {
  it("renders exactly the nine special keys", () => {
    render(<TerminalToolbar onKeyPress={() => {}} />);

    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(9);
  });

  it("no longer offers compose or attach — those live in the ComposeBar", () => {
    render(<TerminalToolbar onKeyPress={() => {}} />);

    expect(screen.queryByRole("button", { name: /attach/i })).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("sends the escape sequence for a plain key", () => {
    const onKeyPress = vi.fn();
    render(<TerminalToolbar onKeyPress={onKeyPress} />);

    fireEvent.click(screen.getByText("esc"));

    expect(onKeyPress).toHaveBeenCalledWith("\x1b");
  });

  it("opens a popover for a menu key instead of sending anything", () => {
    const onKeyPress = vi.fn();
    render(<TerminalToolbar onKeyPress={onKeyPress} />);

    fireEvent.click(screen.getByText("ctrl"));

    expect(onKeyPress).not.toHaveBeenCalled();
    expect(screen.getByText("^C")).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm vitest run src/components/Terminal/TerminalToolbar.test.tsx`

Expected: FAIL — 11 buttons found instead of 9, and the attach button is still present.

- [ ] **Step 3: Strip the toolbar**

In `web/src/components/Terminal/TerminalToolbar.tsx`:

1. Delete the entire `ComposeInput` function (lines 71–190, from the `// Compose modal…` comment through its closing brace).
2. Delete the compose button block and the attachments button block (lines 272–306), leaving the `TOOLBAR_BUTTONS.map(...)` block as the only content of the toolbar `div`.
3. Replace the props interface and the component signature:

```tsx
interface TerminalToolbarProps {
  onKeyPress: (key: string) => void;
}
```

```tsx
export const TerminalToolbar = memo(function TerminalToolbar({
  onKeyPress,
}: TerminalToolbarProps) {
  const [popover, setPopover] = useState<PopoverState | null>(null);
```

Delete the `showCompose` state, the `<ComposeInput ... />` element, and the `if (!visible) return null;` guard — `Terminal` now controls mounting via `isMobile && !selectMode`.

4. Trim the imports to what remains:

```tsx
import { memo, useState } from "react";
import { Globe, CornerDownLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { KeyPopover } from "./KeyPopover";
```

`useCallback`, `useEffect`, `useRef`, `PenLine`, `SendHorizontal`, `Paperclip`, `FilePicker`, and `shellEscape` are all now unused here.

- [ ] **Step 4: Remove the dead `onAttachments` prop**

In `web/src/components/Terminal/index.tsx`, delete the `onAttachments` prop from `TerminalProps` (including its doc comment at `index.tsx:38-39`) and from the destructured parameter list.

In `web/src/components/Workspace/index.tsx`, delete line 261:

```tsx
                      onAttachments={() => setShowFilePicker(true)}
```

Leave the identical prop on `ViewModeRail` (`Workspace/index.tsx:277`) alone — that is the desktop rail and still uses it. Leave `showFilePicker`, `handleFilesPicked`, and the `<FilePicker>` at the bottom of `Workspace` alone for the same reason.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && pnpm vitest run src/components/Terminal/TerminalToolbar.test.tsx`

Expected: PASS, 4 tests.

Run: `cd web && pnpm exec tsc -b`

Expected: no output, exit 0. Any "declared but never read" error here means an import or prop was missed above.

Run: `cd web && pnpm test`

Expected: PASS, all suites.

- [ ] **Step 6: Verify the toolbar on the dev server**

1. `cd web && pnpm dev`, open `http://localhost:5273` with the iPhone 14 Pro device profile.
2. The toolbar shows nine keys and no pen or paperclip.
3. Tap the paperclip **in the compose bar**, pick a file, and confirm the escaped path lands at the caret in the draft rather than being typed into the pane.
4. Confirm `esc`, `ctrl` → `^C`, and the arrows still reach the pane.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/Terminal/TerminalToolbar.tsx web/src/components/Terminal/TerminalToolbar.test.tsx web/src/components/Terminal/index.tsx web/src/components/Workspace/index.tsx
git commit -m "refactor(terminal): drop the compose modal and its toolbar buttons

Compose and attach now live in the persistent ComposeBar, so the toolbar is
back to special keys only — nine of them, which nearly fits a 390px screen
without horizontal scrolling."
```

---

### Task 7: Verify the copy-mode fix end to end

Task 1's unit tests prove the ordering; this proves the behaviour against a real tmux. It is a manual task because an automated version would need a tmux integration test, and production argus runs on this box with live sessions on the default socket.

**Files:** none — verification only.

- [ ] **Step 1: Snapshot the live tmux state first**

Run: `tmux -S ~/.argus/tmux/default ls`

Record the output. Nothing in this task may kill a server or a session; if anything below appears to have disturbed a live session, stop and compare against this snapshot.

- [ ] **Step 2: Restart the dev backend so the Go change is live**

The Go fix is in the node binary, not the SPA, so `pnpm dev` alone will not pick it up. Rebuild and run the node however this repo normally does for development, then point the vite dev server at it (`ARGUS_PORT` selects the backend port the proxy targets — see `web/vite.config.ts`).

- [ ] **Step 3: Reproduce the original bug path**

1. Open `http://localhost:5273` with the iPhone 14 Pro device profile and attach a session running an agent.
2. Drag down on the terminal to scroll up — this sends wheel events and puts the pane into tmux copy-mode. Confirm with:

   `tmux -S ~/.argus/tmux/default display-message -p -t <session> '#{pane_in_mode}'`

   Expected: `1`.
3. Type a distinctive message into the compose bar and send it.

- [ ] **Step 4: Confirm the fix**

Expected, all three:
- The pane snaps back to live output the moment you send.
- `#{pane_in_mode}` now reports `0`.
- The message arrives in the agent's input intact — not swallowed, and with no stray copy-mode side effects (no search prompt, no selection left behind).

Re-run the command from Step 3.2 to check `pane_in_mode`.

- [ ] **Step 5: Confirm the unattached-tab path**

1. Open a new tab and do **not** attach a session — this connects to `/ws/terminal`, a plain `$SHELL` with no tmux.
2. Run something that produces a screenful of output (`seq 1 200`), drag up to scroll into history, then send `echo hello` from the compose bar.
3. The view must jump to the bottom so `hello` is visible. If it stays parked in history, `scrollToBottom` is not wired up.

- [ ] **Step 6: Confirm no live sessions were disturbed**

Run: `tmux -S ~/.argus/tmux/default ls`

Compare against the Step 1 snapshot — the same sessions must still be present.

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Focus model: two live targets | 5 (both bars mount alongside a focusable xterm; manual step 4.7) |
| Layout: both bars permanent | 5 |
| Growth: overlay, not resize | 4 (CSS growth) + 5 (transform) |
| Scroll compensation via `translateY` | 5 |
| Send semantics (Return = newline, ▶ sends, clears, keeps focus) | 3 |
| Disabled while disconnected, draft preserved | 3 |
| Attachments move into the input bar | 2 + 3 |
| Focus affordance | 3 |
| Draft scope (per-tab, lost on reload) | 3 (component state; `Terminal` stays mounted per tab) |
| Select mode hides both bars | 5 (`isMobile && !selectMode`) |
| Attached sessions: exit tmux copy-mode | 1, verified in 7 |
| Unattached tabs: scroll to bottom | 5, verified in 7 |
| Scope line: raw keys stay passthrough | 1 (only the `"text"` message cancels) |
| Toolbar keeps its height; nine keys | 6 |
| File removals and prop cleanup | 6 |

Two deliberate departures from the spec, both flagged in place:

- **Send-button visibility** (Task 3): the spec's table says "hidden" when the terminal has focus; the implemented rule is "hidden only when unfocused *and* empty", so a draft stays sendable after tapping away to press a special key.
- **Testing** (Tasks 4, 5, 7): the spec called for a `Terminal`-level test asserting the transform tracks the overlay height. `Terminal` cannot mount in jsdom (xterm needs canvas/WebGL plus a live socket), so that assertion moved down into `ComposeBar` — where the height conversion is real logic a stubbed `ResizeObserver` can drive — and the pixel-level claim became a concrete manual check (`stty size` unchanged across a grow) in Task 5.
