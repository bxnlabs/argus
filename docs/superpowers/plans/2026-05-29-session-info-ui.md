# Session Info UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the web session-info dialog (left-aligned header with pin + provider brand logo, a status/time/profile caption with a timestamp tooltip, copyable location boxes) and add provider brand logos to the sidebar rows.

**Architecture:** Pure presentation changes on the web client, reading data the app already holds (the `Session` object and runtime status map). Shared leaf pieces (a clipboard helper, status helpers, a `ProviderLogo` component, a `CopyableField` component) are built first, then the dialog and sidebar consume them. No backend, API, or data-model changes. The git-changes metric from the original issue is out of scope.

**Tech Stack:** React 19 + TypeScript, Vite, Tailwind, Radix UI (dialog/tooltip), lucide-react icons, sonner toasts, Vitest + @testing-library/react (jsdom). Package manager: pnpm. All commands run from `web/`.

**Spec:** `docs/superpowers/specs/2026-05-29-session-info-ui-design.md`

**Conventions observed in this repo:**
- Pure helpers get Vitest unit tests; presentational/interactive components are verified in the browser (no heavy render tests). Follow this — do not force brittle tests onto interactive components.
- Run a single test file with `pnpm exec vitest run <path>`. Run the whole suite with `pnpm test`. Typecheck + production build with `pnpm build` (`tsc -b && vite build`).
- **tmux safety:** production argus runs on this machine on the **default** tmux socket with live sessions. Browser verification must only *view* existing sessions — never create, attach, kill, or otherwise mutate sessions, and never run `tmux kill-server`.

---

## Task 1: Clipboard helper

**Files:**
- Create: `web/src/lib/clipboard.ts`
- Test: `web/src/lib/clipboard.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/lib/clipboard.test.ts`:

```ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { copyToClipboard } from "./clipboard";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("copyToClipboard", () => {
  it("uses the async clipboard API when available", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    const ok = await copyToClipboard("hello");
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("falls back to execCommand when the clipboard API is unavailable", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });
    if (typeof document.execCommand !== "function") {
      (document as unknown as { execCommand: () => boolean }).execCommand =
        () => false;
    }
    const exec = vi.spyOn(document, "execCommand").mockReturnValue(true);
    const ok = await copyToClipboard("hi");
    expect(ok).toBe(true);
    expect(exec).toHaveBeenCalledWith("copy");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/lib/clipboard.test.ts`
Expected: FAIL — `Failed to resolve import "./clipboard"` (file does not exist yet).

- [ ] **Step 3: Implement the helper**

Create `web/src/lib/clipboard.ts`:

```ts
// copyToClipboard copies text using the async Clipboard API, falling back to a
// hidden-textarea execCommand for contexts where it is unavailable (mirrors the
// pattern in Terminal/hooks/terminal-init.ts). Returns whether the copy is
// believed to have succeeded, so callers can show success/failure feedback.
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or insecure context — fall through to execCommand.
    }
  }
  return execCommandCopy(text);
}

function execCommandCopy(text: string): boolean {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  document.body.removeChild(textarea);
  return ok;
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/lib/clipboard.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/clipboard.ts web/src/lib/clipboard.test.ts
git commit -m "feat(web): add copyToClipboard helper"
```

---

## Task 2: Shared session-status helpers

Extract the three status helpers currently private to `SessionList` into a shared module so the dialog caption can match the sidebar exactly.

**Files:**
- Create: `web/src/lib/sessionStatus.ts`
- Create: `web/src/lib/sessionStatus.test.ts`
- Modify: `web/src/components/SessionList/index.tsx` (remove local copies, import from the new module)

- [ ] **Step 1: Write the failing test**

Create `web/src/lib/sessionStatus.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import {
  getStatusColor,
  getStatusLabel,
  getStatusAnimation,
} from "./sessionStatus";

describe("sessionStatus helpers", () => {
  it("maps known statuses to colors", () => {
    expect(getStatusColor("active")).toBe("bg-green-500");
    expect(getStatusColor("idle")).toBe("bg-muted-foreground");
    expect(getStatusColor("dead")).toBe("bg-red-500/50");
    expect(getStatusColor(undefined)).toBe("bg-muted-foreground/40");
  });

  it("labels known statuses and returns empty string otherwise", () => {
    expect(getStatusLabel("active")).toBe("Active");
    expect(getStatusLabel("idle")).toBe("Idle");
    expect(getStatusLabel("dead")).toBe("Dead");
    expect(getStatusLabel(undefined)).toBe("");
  });

  it("animates only the active status", () => {
    expect(getStatusAnimation("active")).toBe("animate-pulse-green");
    expect(getStatusAnimation("idle")).toBe("");
    expect(getStatusAnimation(undefined)).toBe("");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/lib/sessionStatus.test.ts`
Expected: FAIL — cannot resolve `./sessionStatus`.

- [ ] **Step 3: Create the shared module**

Create `web/src/lib/sessionStatus.ts` (verbatim moves of the functions currently in `SessionList/index.tsx`):

```ts
export function getStatusColor(status?: string) {
  switch (status) {
    case "active":
      return "bg-green-500";
    case "idle":
      return "bg-muted-foreground";
    case "dead":
      return "bg-red-500/50";
    default:
      return "bg-muted-foreground/40";
  }
}

export function getStatusAnimation(status?: string) {
  switch (status) {
    case "active":
      return "animate-pulse-green";
    default:
      return "";
  }
}

export function getStatusLabel(status?: string) {
  switch (status) {
    case "active":
      return "Active";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    default:
      return "";
  }
}
```

- [ ] **Step 4: Remove the local copies from `SessionList/index.tsx`**

Delete the three function definitions `getStatusColor`, `getStatusAnimation`, and `getStatusLabel` (currently at lines 17–50, the block between the imports and the `partitionSessions` comment).

Then add this import alongside the existing `@/lib/utils` import near the top of the file:

```ts
import {
  getStatusColor,
  getStatusAnimation,
  getStatusLabel,
} from "@/lib/sessionStatus";
```

- [ ] **Step 5: Run the new test and the existing SessionList test**

Run: `cd web && pnpm exec vitest run src/lib/sessionStatus.test.ts src/components/SessionList/index.test.tsx`
Expected: PASS (both files).

- [ ] **Step 6: Typecheck**

Run: `cd web && pnpm build`
Expected: build succeeds (confirms `SessionList` still compiles after the move).

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/sessionStatus.ts web/src/lib/sessionStatus.test.ts web/src/components/SessionList/index.tsx
git commit -m "refactor(web): extract session status helpers into shared module"
```

---

## Task 3: ProviderLogo component

A shared component rendering a brand mark per provider. Brand path/color data is vendored inline from simple-icons (a real data-fetch step, not a placeholder).

**Files:**
- Create: `web/src/components/ProviderLogo.tsx`
- Test: `web/src/components/ProviderLogo.test.tsx`

- [ ] **Step 1: Fetch the brand mark data**

Each simple-icons SVG is `<svg ...><title>Name</title><path d="..."/></svg>` on a `0 0 24 24` viewBox. Fetch these three files and extract the single `d` attribute from each (use WebFetch, or `curl`):

- Claude: `https://raw.githubusercontent.com/simple-icons/simple-icons/develop/icons/claude.svg`
- Codex (OpenAI mark): `https://raw.githubusercontent.com/simple-icons/simple-icons/develop/icons/openai.svg`
- Gemini: `https://raw.githubusercontent.com/simple-icons/simple-icons/develop/icons/googlegemini.svg`

For each, also note the brand hex shown on `https://simpleicons.org` for that slug. Suggested values (confirm against the source, the exact shade is a visual detail to verify in the browser): Claude `#D97757`, OpenAI `#000000`, Gemini `#886FB5`. Record the three `d` strings and three hex values for Step 3.

- [ ] **Step 2: Write the failing test**

Create `web/src/components/ProviderLogo.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { ProviderLogo } from "./ProviderLogo";

describe("ProviderLogo", () => {
  it("renders a labeled brand <svg> for a known provider", () => {
    const { container } = render(<ProviderLogo type="claude" />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("aria-label")).toBe("Claude");
  });

  it("falls back to a terminal glyph for shell", () => {
    const { container } = render(<ProviderLogo type="shell" />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("class") ?? "").toContain("lucide-terminal");
  });

  it("applies the provided className for sizing", () => {
    const { container } = render(
      <ProviderLogo type="gemini" className="h-4 w-4" />,
    );
    expect(container.querySelector("svg")?.getAttribute("class") ?? "").toContain(
      "h-4",
    );
  });
});
```

- [ ] **Step 3: Implement the component**

Create `web/src/components/ProviderLogo.tsx`. Paste the three `d` strings and hex values from Step 1 into the `BRAND` map where indicated:

```tsx
import { Terminal } from "lucide-react";
import type { ProviderType } from "@/types";
import { cn } from "@/lib/utils";

interface BrandMark {
  label: string;
  hex: string;
  path: string;
}

// Brand marks vendored from simple-icons (https://simpleicons.org): single-path
// 24x24 marks. `path` is the `d` attribute and `hex` the brand color, copied
// verbatim from each icon's source (slugs: claude, openai, googlegemini).
const BRAND: Partial<Record<ProviderType, BrandMark>> = {
  claude: {
    label: "Claude",
    hex: "#D97757",
    path: "PASTE_CLAUDE_PATH_FROM_STEP_1",
  },
  codex: {
    label: "Codex",
    hex: "#000000",
    path: "PASTE_OPENAI_PATH_FROM_STEP_1",
  },
  gemini: {
    label: "Gemini",
    hex: "#886FB5",
    path: "PASTE_GEMINI_PATH_FROM_STEP_1",
  },
};

interface ProviderLogoProps {
  type: ProviderType;
  className?: string;
}

// ProviderLogo renders the provider's brand mark (Claude/Codex/Gemini) or a
// terminal glyph for shell and any unknown provider. Size via className.
export function ProviderLogo({ type, className }: ProviderLogoProps) {
  const mark = BRAND[type];
  if (!mark) {
    return (
      <Terminal
        role="img"
        aria-label="Terminal"
        className={cn("text-muted-foreground", className)}
      />
    );
  }
  return (
    <svg
      role="img"
      aria-label={mark.label}
      viewBox="0 0 24 24"
      fill={mark.hex}
      className={className}
    >
      <path d={mark.path} />
    </svg>
  );
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/components/ProviderLogo.test.tsx`
Expected: PASS (3 tests). If the shell test fails on the `lucide-terminal` class, confirm the lucide version still emits that class via `container.querySelector("svg")?.getAttribute("class")` and adjust the assertion to the emitted class name.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ProviderLogo.tsx web/src/components/ProviderLogo.test.tsx
git commit -m "feat(web): add ProviderLogo brand-mark component"
```

---

## Task 4: CopyableField component

A presentational/interactive field with click-to-copy. Verified via the dialog in the browser (no unit test — consistent with repo precedent for interactive components).

**Files:**
- Create: `web/src/components/SessionInfoDialog/CopyableField.tsx`

- [ ] **Step 1: Implement the component**

Create `web/src/components/SessionInfoDialog/CopyableField.tsx`:

```tsx
import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { copyToClipboard } from "@/lib/clipboard";

interface CopyableFieldProps {
  label: string;
  // What the user sees (e.g. a tilde-contracted path).
  displayValue: string;
  // What gets copied; defaults to displayValue (e.g. the full absolute path).
  copyValue?: string;
  // Inline layout (label + value on one row) instead of a boxed field.
  inline?: boolean;
}

export function CopyableField({
  label,
  displayValue,
  copyValue,
  inline = false,
}: CopyableFieldProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    const ok = await copyToClipboard(copyValue ?? displayValue);
    if (ok) {
      setCopied(true);
      toast.success("Copied");
      setTimeout(() => setCopied(false), 1200);
    } else {
      toast.error("Copy failed");
    }
  };

  const Icon = copied ? Check : Copy;
  const copyButton = (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={`Copy ${label}`}
      className="text-muted-foreground hover:text-foreground flex-shrink-0"
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  );

  if (inline) {
    return (
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground w-20 flex-shrink-0">{label}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs">
          {displayValue}
        </span>
        {copyButton}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="bg-muted/50 flex items-center gap-2 rounded-md border px-2 py-1.5">
        <span className="min-w-0 flex-1 break-all font-mono text-xs">
          {displayValue}
        </span>
        {copyButton}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && pnpm build`
Expected: build succeeds (the component is standalone; `sonner` and `lucide-react` are existing deps).

- [ ] **Step 3: Commit**

```bash
git add web/src/components/SessionInfoDialog/CopyableField.tsx
git commit -m "feat(web): add CopyableField click-to-copy field"
```

---

## Task 5: Rebuild the info dialog (view-model + dialog)

`fields.ts` and `index.tsx` are tightly coupled (the view-model and its sole renderer), so they change together to keep the build green. The view-model builder is pure and TDD-tested; the dialog itself is presentational and verified in the browser.

**Files:**
- Modify: `web/src/components/SessionInfoDialog/fields.ts` (full rewrite)
- Modify: `web/src/components/SessionInfoDialog/fields.test.ts` (full rewrite)
- Modify: `web/src/components/SessionInfoDialog/index.tsx` (full rewrite)

- [ ] **Step 1: Rewrite the test for the new view-model**

Replace the entire contents of `web/src/components/SessionInfoDialog/fields.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import { buildSessionInfoModel } from "./fields";
import type { Session } from "@/types";

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "id",
    name: "name",
    tmux_name: "claude-id",
    created_at: "2026-05-20 14:32:05",
    updated_at: "2026-05-28 09:15:00",
    working_directory: "/home/u/work",
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

describe("buildSessionInfoModel", () => {
  it("uses working_directory as the directory and omits worktreeDir for plain sessions", () => {
    const m = buildSessionInfoModel(makeSession(), "idle", "/home/u");
    expect(m.location.directory.copy).toBe("/home/u/work");
    expect(m.location.directory.display).toBe("~/work");
    expect(m.location.worktreeDir).toBeNull();
  });

  it("uses git_parent_dir as directory and working_directory as worktreeDir for worktree sessions", () => {
    const m = buildSessionInfoModel(
      makeSession({
        git_parent_dir: "/home/u/work",
        working_directory: "/home/u/.wt/bxn-104",
        worktree_branch: "jeev/bxn-104",
      }),
      "active",
      "/home/u",
    );
    expect(m.location.directory.copy).toBe("/home/u/work");
    expect(m.location.directory.display).toBe("~/work");
    expect(m.location.worktreeDir?.copy).toBe("/home/u/.wt/bxn-104");
    expect(m.location.worktreeDir?.display).toBe("~/.wt/bxn-104");
    expect(m.location.branch).toBe("jeev/bxn-104");
  });

  it("omits repo, branch, and model when absent and passes status/profile through", () => {
    const m = buildSessionInfoModel(makeSession(), undefined, "/home/u");
    expect(m.location.repo).toBeNull();
    expect(m.location.branch).toBeNull();
    expect(m.details.model).toBeNull();
    expect(m.status).toBeUndefined();
    expect(m.profile).toBeNull();
  });

  it("includes model, repo, branch, profile, and pinned when present", () => {
    const m = buildSessionInfoModel(
      makeSession({
        model: "claude-opus-4-7",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "jeev/bxn-97",
        profile: "default",
        pinned: true,
      }),
      "idle",
      "/home/u",
    );
    expect(m.details.model).toBe("claude-opus-4-7");
    expect(m.location.repo).toBe("bxnlabs/argus");
    expect(m.location.branch).toBe("jeev/bxn-97");
    expect(m.profile).toBe("default");
    expect(m.pinned).toBe(true);
  });

  it("exposes absolute timestamps and a relative updated time", () => {
    const m = buildSessionInfoModel(makeSession(), "idle", "/home/u");
    expect(m.createdAbsolute).toBe("2026-05-20 14:32:05");
    expect(m.updatedAbsolute).toBe("2026-05-28 09:15:00");
    expect(typeof m.updatedRelative).toBe("string");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/components/SessionInfoDialog/fields.test.ts`
Expected: FAIL — `buildSessionInfoModel` is not exported (the file still exports `buildSessionInfoSections`).

- [ ] **Step 3: Rewrite `fields.ts`**

Replace the entire contents of `web/src/components/SessionInfoDialog/fields.ts`:

```ts
import type { ProviderType, Session } from "@/types";
import { contractTilde, formatRelativeTime, parseRepoFromRemoteURL } from "@/lib/utils";

export interface CopyableValue {
  display: string;
  copy: string;
}

export interface SessionInfoModel {
  name: string;
  pinned: boolean;
  providerType: ProviderType;
  status: string | undefined;
  updatedRelative: string;
  createdAbsolute: string;
  updatedAbsolute: string;
  profile: string | null;
  details: { id: string; model: string | null };
  location: {
    directory: CopyableValue;
    repo: string | null;
    branch: string | null;
    worktreeDir: CopyableValue | null;
  };
}

// buildSessionInfoModel produces the view-model rendered by the session info
// dialog. Directory is the main repo root (git_parent_dir) when the session
// runs in a worktree, otherwise the working directory. worktreeDir is the
// session's working directory, shown only for worktree sessions (git_parent_dir
// set and distinct from working_directory) to avoid a duplicate of Directory.
// Paths carry a tilde-contracted display and a full absolute copy value.
export function buildSessionInfoModel(
  session: Session,
  status: string | undefined,
  homeDir: string,
): SessionInfoModel {
  const repo = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;

  const isWorktree =
    !!session.git_parent_dir &&
    session.git_parent_dir !== session.working_directory;

  const directoryPath = session.git_parent_dir ?? session.working_directory;

  return {
    name: session.name || "Session",
    pinned: session.pinned,
    providerType: session.provider_type,
    status,
    updatedRelative: formatRelativeTime(session.updated_at),
    createdAbsolute: session.created_at,
    updatedAbsolute: session.updated_at,
    profile: session.profile,
    details: { id: session.id, model: session.model },
    location: {
      directory: {
        display: contractTilde(directoryPath, homeDir),
        copy: directoryPath,
      },
      repo,
      branch: session.worktree_branch,
      worktreeDir: isWorktree
        ? {
            display: contractTilde(session.working_directory, homeDir),
            copy: session.working_directory,
          }
        : null,
    },
  };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/components/SessionInfoDialog/fields.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Rewrite `index.tsx`**

Replace the entire contents of `web/src/components/SessionInfoDialog/index.tsx`:

```tsx
import { Pin } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProviderLogo } from "@/components/ProviderLogo";
import {
  getStatusAnimation,
  getStatusColor,
  getStatusLabel,
} from "@/lib/sessionStatus";
import { cn } from "@/lib/utils";
import type { Session } from "@/types";
import { buildSessionInfoModel } from "./fields";
import { CopyableField } from "./CopyableField";

interface SessionInfoDialogProps {
  session: Session | null;
  status?: string;
  homeDir: string;
  onClose: () => void;
}

export function SessionInfoDialog({
  session,
  status,
  homeDir,
  onClose,
}: SessionInfoDialogProps) {
  const model = session ? buildSessionInfoModel(session, status, homeDir) : null;

  return (
    <Dialog open={session !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        {model && (
          <>
            <DialogHeader className="text-left">
              <DialogTitle asChild>
                <div className="flex min-w-0 items-center gap-2">
                  {model.pinned && (
                    <Pin className="h-4 w-4 flex-shrink-0 fill-current" />
                  )}
                  <span className="min-w-0 flex-1 truncate">{model.name}</span>
                  <ProviderLogo
                    type={model.providerType}
                    className="h-4 w-4 flex-shrink-0"
                  />
                </div>
              </DialogTitle>
              <DialogDescription asChild>
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                      getStatusColor(model.status),
                      getStatusAnimation(model.status),
                    )}
                  />
                  <span>
                    {getStatusLabel(model.status) || "Unknown"}
                    {" · "}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="cursor-default underline decoration-dotted underline-offset-2">
                          {model.updatedRelative}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        <div>Created: {model.createdAbsolute}</div>
                        <div>Updated: {model.updatedAbsolute}</div>
                      </TooltipContent>
                    </Tooltip>
                    {model.profile ? ` · ${model.profile}` : ""}
                  </span>
                </div>
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-1">
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  Details
                </div>
                <CopyableField label="ID" displayValue={model.details.id} inline />
                {model.details.model && (
                  <div className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground w-20 flex-shrink-0">
                      Model
                    </span>
                    <span className="min-w-0 flex-1 truncate">
                      {model.details.model}
                    </span>
                  </div>
                )}
              </div>

              <div className="space-y-2">
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  Location
                </div>
                <CopyableField
                  label="Directory"
                  displayValue={model.location.directory.display}
                  copyValue={model.location.directory.copy}
                />
                {model.location.repo && (
                  <CopyableField
                    label="Repo"
                    displayValue={model.location.repo}
                  />
                )}
                {model.location.branch && (
                  <CopyableField
                    label="Branch"
                    displayValue={model.location.branch}
                  />
                )}
                {model.location.worktreeDir && (
                  <CopyableField
                    label="Worktree dir"
                    displayValue={model.location.worktreeDir.display}
                    copyValue={model.location.worktreeDir.copy}
                  />
                )}
              </div>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 6: Typecheck + run the dialog's test**

Run: `cd web && pnpm build && pnpm exec vitest run src/components/SessionInfoDialog/fields.test.ts`
Expected: build succeeds and the 5 field tests pass. (Build also confirms no remaining references to the old `buildSessionInfoSections`.)

- [ ] **Step 7: Commit**

```bash
git add web/src/components/SessionInfoDialog/fields.ts web/src/components/SessionInfoDialog/fields.test.ts web/src/components/SessionInfoDialog/index.tsx
git commit -m "feat(web): redesign session info dialog (header, caption, copyable fields)"
```

---

## Task 6: Provider logo in sidebar rows

**Files:**
- Modify: `web/src/components/SessionList/index.tsx` (name line in `SessionItem`)

- [ ] **Step 1: Add the import**

In `web/src/components/SessionList/index.tsx`, add near the other component imports:

```ts
import { ProviderLogo } from "@/components/ProviderLogo";
```

- [ ] **Step 2: Add the logo to the name line**

In `SessionItem`, replace the name block (currently):

```tsx
            <div className="flex min-w-0 items-center gap-1">
              <span className="truncate text-sm">
                {session.name || "Unnamed Session"}
              </span>
            </div>
```

with:

```tsx
            <div className="flex min-w-0 items-center gap-1.5">
              <span className="min-w-0 flex-1 truncate text-sm">
                {session.name || "Unnamed Session"}
              </span>
              <ProviderLogo
                type={session.provider_type}
                className="h-3.5 w-3.5 flex-shrink-0"
              />
            </div>
```

- [ ] **Step 3: Typecheck + run the SessionList test**

Run: `cd web && pnpm build && pnpm exec vitest run src/components/SessionList/index.test.tsx`
Expected: build succeeds and the SessionList tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/SessionList/index.tsx
git commit -m "feat(web): show provider logo on sidebar session rows"
```

---

## Task 7: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full unit suite**

Run: `cd web && pnpm test`
Expected: all tests pass.

- [ ] **Step 2: Typecheck + production build**

Run: `cd web && pnpm build`
Expected: `tsc -b` reports no errors and `vite build` completes.

- [ ] **Step 3: Format the new/changed files**

Run: `cd web && pnpm exec prettier --write src/lib/clipboard.ts src/lib/clipboard.test.ts src/lib/sessionStatus.ts src/lib/sessionStatus.test.ts src/components/ProviderLogo.tsx src/components/ProviderLogo.test.tsx src/components/SessionInfoDialog/CopyableField.tsx src/components/SessionInfoDialog/fields.ts src/components/SessionInfoDialog/fields.test.ts src/components/SessionInfoDialog/index.tsx src/components/SessionList/index.tsx`
Expected: prettier rewrites any files not already conformant. If it changes anything, re-run Step 2, then commit with `style(web): prettier formatting`.

- [ ] **Step 4: Browser verification**

Start the dev web server (`cd web && pnpm run dev`, or `make dev-web` from the repo root) and open the local URL. **Do not create, attach to, kill, or otherwise mutate sessions** — production argus runs on the default tmux socket on this machine; only view existing sessions.

Verify:
- **Sidebar:** each session row shows the correct brand logo at the right edge of the name (claude/codex/gemini marks; terminal glyph for shell). Long names truncate without pushing the logo off-row.
- **Dialog header:** open a session's `...` → Info. The header is left-aligned on both a narrow (mobile-width) and wide window. A pinned session shows the pin icon before the name; the provider logo sits to the right of the name.
- **Caption:** shows `status · relative-time · profile` (profile segment absent when the session has none); the status dot color/animation matches the sidebar. Hovering the relative time shows a tooltip with full Created/Updated timestamps.
- **Body:** `DETAILS` shows ID (with a working copy button) and Model (only when set). `LOCATION` shows Directory, Repo (if any), Branch (if any), and — for a worktree session — Worktree dir. A plain (non-worktree) session shows only Directory (no Worktree dir, no Branch). Each copy button copies the value (paste to confirm the full absolute path for directory/worktree) and shows the check-icon + "Copied" toast.
- Confirm Auto-approve and the standalone Timestamps section are gone.

- [ ] **Step 5: Final commit (if Step 3 or browser fixes changed anything)**

```bash
git add -A web/
git commit -m "style(web): formatting and verification fixes for session info UI"
```

(Skip if nothing changed after Task 6.)

---

## Self-Review Notes

- **Spec coverage:** left-aligned header + pin icon + provider logo (Task 5); status/time/profile caption with timestamp tooltip (Task 5); copyable location boxes with the agreed mapping (Tasks 4–5); sidebar provider logo (Task 6); dropped Auto-approve / Provider-type row / Timestamps section (Task 5); shared status helpers and clipboard helper (Tasks 1–2); brand logos sourced from simple-icons (Task 3). Git-changes metric intentionally absent (punted in spec).
- **No sidebar pin icon** — intentionally excluded per spec (commit `97ef53a` removed it deliberately).
- **Type consistency:** `buildSessionInfoModel` → `SessionInfoModel` (with `CopyableValue`) is produced in Task 5 and consumed by the dialog in the same task; `ProviderLogo`'s `{ type, className }` props match every call site (dialog `h-4 w-4`, sidebar `h-3.5 w-3.5`); `CopyableField`'s `{ label, displayValue, copyValue?, inline? }` match all dialog usages; `copyToClipboard` and the three status helpers keep identical signatures across producer and consumers.
- **Brand path data** (Task 3, Step 1) is the one externally-sourced input; it is fetched from named URLs during implementation rather than fabricated, since brand vector data cannot be reproduced accurately from memory.
