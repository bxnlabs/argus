import type { Session } from "@/types";
import { contractTilde, parseRepoFromRemoteURL } from "@/lib/utils";

export interface CopyableValue {
  display: string;
  copy: string;
}

export interface SessionLocation {
  directory: CopyableValue;
  repo: string | null;
  branch: string | null;
  worktreeDir: CopyableValue | null;
}

// getSessionLocation derives the location fields shown in the session info
// dialog. Directory is the main repo root (git_parent_dir) when the session
// runs in a worktree, otherwise the working directory. worktreeDir is the
// session's working directory, shown only for worktree sessions (git_parent_dir
// set and distinct from working_directory) to avoid a duplicate of Directory.
// Paths carry a tilde-contracted display and a full absolute copy value.
export function getSessionLocation(
  session: Session,
  homeDir: string,
): SessionLocation {
  const repo = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;

  const isWorktree =
    !!session.git_parent_dir &&
    session.git_parent_dir !== session.working_directory;

  const directoryPath = session.git_parent_dir ?? session.working_directory;

  return {
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
  };
}
