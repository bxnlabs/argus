import { describe, it, expect, afterEach, beforeAll } from "vitest";
import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/react";
import { SessionInfoDialog } from "./index";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { Session } from "@/types";

beforeAll(() => {
  if (!("ResizeObserver" in globalThis)) {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess-123",
    name: "My Session",
    tmux_name: "claude-sess-123",
    created_at: "2026-01-01 00:00:00",
    updated_at: "2026-01-01 00:00:00",
    working_directory: "/home/user/project",
    worktree_branch: null,
    git_parent_dir: null,
    git_remote_url: null,
    provider_session_id: null,
    model: null,
    system_prompt: null,
    provider_type: "claude",
    auto_approve: false,
    profile: null,
    pinned: false,
    ...overrides,
  };
}

function renderDialog() {
  return render(
    <TooltipProvider>
      <SessionInfoDialog
        session={makeSession()}
        status="idle"
        homeDir="/home/user"
        onClose={() => {}}
      />
    </TooltipProvider>,
  );
}

function timestampTrigger(): HTMLElement {
  const el = document.querySelector(
    "span.underline.decoration-dotted",
  ) as HTMLElement | null;
  if (!el) throw new Error("timestamp tooltip trigger not found");
  return el;
}

afterEach(cleanup);

describe("SessionInfoDialog timestamp tooltip", () => {
  it("keeps the created/updated timestamps hidden when the dialog opens", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    // Wait until Radix's open-auto-focus has run (focus leaves the body).
    await waitFor(() => expect(document.activeElement).not.toBe(document.body));
    // The dialog must not auto-focus the trigger (that would pop the tooltip).
    expect(document.activeElement).not.toBe(timestampTrigger());
    expect(screen.queryAllByText(/^Created:/)).toHaveLength(0);
    expect(screen.queryAllByText(/^Updated:/)).toHaveLength(0);
  });

  it("reveals the timestamps once the trigger is focused (keyboard a11y)", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    fireEvent.focus(timestampTrigger());
    await waitFor(() =>
      expect(screen.getAllByText(/^Created:/).length).toBeGreaterThan(0),
    );
    expect(screen.getAllByText(/^Updated:/).length).toBeGreaterThan(0);
  });
});

describe("SessionInfoDialog profile field", () => {
  it("renders the profile in the Details section, not in the description", async () => {
    render(
      <TooltipProvider>
        <SessionInfoDialog
          session={makeSession({ profile: "review" })}
          status="idle"
          homeDir="/home/user"
          onClose={() => {}}
        />
      </TooltipProvider>,
    );
    await screen.findByRole("dialog");

    // A labeled "Profile" field exists (Details fields use labels; the
    // description does not).
    expect(screen.getByText("Profile")).toBeTruthy();
    expect(screen.getByText("review")).toBeTruthy();

    // The profile must no longer be appended to the status/description line.
    const description = document.querySelector(
      '[data-slot="dialog-description"]',
    ) as HTMLElement | null;
    expect(description).not.toBeNull();
    expect(description!.textContent).not.toContain("review");
  });

  it("omits the Profile field when no profile is set", async () => {
    render(
      <TooltipProvider>
        <SessionInfoDialog
          session={makeSession({ profile: null })}
          status="idle"
          homeDir="/home/user"
          onClose={() => {}}
        />
      </TooltipProvider>,
    );
    await screen.findByRole("dialog");
    expect(screen.queryByText("Profile")).toBeNull();
  });
});
