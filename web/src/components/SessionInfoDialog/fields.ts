import type { Session } from "@/types";
import { compressPath, formatRelativeTime, parseRepoFromRemoteURL } from "@/lib/utils";

export interface InfoRow {
  label: string;
  value: string;
}

export interface InfoSection {
  title: string | null;
  rows: InfoRow[];
}

function statusLabel(status: string | undefined): string {
  switch (status) {
    case "active":
      return "Active";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    default:
      return "Unknown";
  }
}

// buildSessionInfoSections produces the curated, grouped fields shown in the
// session info dialog. Optional fields (model, repo, branch) are omitted when
// absent. status is the runtime status string, or undefined when unavailable.
export function buildSessionInfoSections(
  session: Session,
  status: string | undefined,
  homeDir: string,
): InfoSection[] {
  const repo = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;
  const dir = compressPath(
    session.git_parent_dir ?? session.working_directory,
    homeDir,
    60,
  );

  const header: InfoRow[] = [
    { label: "ID", value: session.id },
    { label: "Status", value: statusLabel(status) },
    { label: "Pinned", value: session.pinned ? "Yes" : "No" },
    { label: "Profile", value: session.profile ?? "None" },
  ];

  const provider: InfoRow[] = [{ label: "Type", value: session.provider_type }];
  if (session.model) provider.push({ label: "Model", value: session.model });
  provider.push({
    label: "Auto-approve",
    value: session.auto_approve ? "On" : "Off",
  });

  const location: InfoRow[] = [{ label: "Directory", value: dir }];
  if (repo) location.push({ label: "Repo", value: repo });
  if (session.worktree_branch)
    location.push({ label: "Branch", value: session.worktree_branch });

  const timestamps: InfoRow[] = [
    {
      label: "Created",
      value: `${session.created_at} (${formatRelativeTime(session.created_at)})`,
    },
    {
      label: "Updated",
      value: `${session.updated_at} (${formatRelativeTime(session.updated_at)})`,
    },
  ];

  return [
    { title: null, rows: header },
    { title: "Provider", rows: provider },
    { title: "Location", rows: location },
    { title: "Timestamps", rows: timestamps },
  ];
}
