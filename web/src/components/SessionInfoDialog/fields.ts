import type { ProviderType, Session } from "@/types";
import {
  contractTilde,
  formatRelativeTime,
  parseRepoFromRemoteURL,
} from "@/lib/utils";

export interface CopyableValue {
  display: string;
  copy: string;
}

export interface SessionInfoModel {
  name: string;
  providerType: ProviderType;
  autoApprove: boolean;
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
    providerType: session.provider_type,
    autoApprove: session.auto_approve,
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
