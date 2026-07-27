import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { SessionItem } from "./index";
import type { Session } from "@/types";
import type { BusyKind } from "@/hooks/useSessionMutationState";

afterEach(cleanup);

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess-1",
    name: "My Session",
    tmux_name: "claude-sess-1",
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

function renderItem(busy?: BusyKind) {
  const onAttachSession = vi.fn();
  const { container } = render(
    <SessionItem
      session={makeSession()}
      homeDir="/home/user"
      isActive={false}
      statusValue="idle"
      unreadSince={null}
      userMarkedUnreadAt={null}
      minuteTick={0}
      isRenaming={false}
      renameValue=""
      renameInputRef={() => {}}
      onRenameValueChange={() => {}}
      onConfirmRename={() => {}}
      onCancelRename={() => {}}
      onStartRename={() => {}}
      onAttachSession={onAttachSession}
      onDeleteSession={() => {}}
      onCloneSession={() => {}}
      onChangeProfile={() => {}}
      onViewInfo={() => {}}
      canChangeProfile={false}
      onTogglePin={() => {}}
      onMarkRead={() => {}}
      onMarkUnread={() => {}}
      renamePendingRef={{ current: false }}
      busy={busy}
    />,
  );
  // SessionItem's root element is the row div — not portaled, so container works.
  const row = container.firstElementChild as HTMLElement;
  return { onAttachSession, row, container };
}

describe("SessionItem busy state", () => {
  it("renders a spinner and the busy label while deleting", () => {
    const { container } = renderItem("deleting");
    expect(container.textContent).toContain("Deleting…");
    expect(container.querySelector(".animate-spin")).not.toBeNull();
  });

  it("marks the row aria-busy and dims it", () => {
    const { row } = renderItem("deleting");
    expect(row.getAttribute("aria-busy")).toBe("true");
    expect(row.className).toContain("opacity-60");
  });

  it("hides the actions menu while busy", () => {
    renderItem("deleting");
    expect(screen.queryByLabelText("Session actions")).toBeNull();
  });

  it("ignores clicks while busy", () => {
    const { onAttachSession, row } = renderItem("deleting");
    fireEvent.click(row);
    expect(onAttachSession).not.toHaveBeenCalled();
  });

  it("behaves normally when not busy", () => {
    const { onAttachSession, row, container } = renderItem(undefined);
    expect(row.getAttribute("aria-busy")).toBeNull();
    expect(container.querySelector(".animate-spin")).toBeNull();
    expect(screen.queryByLabelText("Session actions")).not.toBeNull();
    fireEvent.click(row);
    expect(onAttachSession).toHaveBeenCalledWith("sess-1");
  });
});
