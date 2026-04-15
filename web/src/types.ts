export interface Session {
  id: string;
  name: string;
  tmux_name: string;
  created_at: string;
  updated_at: string;
  working_directory: string;
  worktree_branch: string | null;
  git_parent_dir: string | null;
  git_remote_url: string | null;
  provider_session_id: string | null;
  model: string | null;
  system_prompt: string | null;
  provider_type: ProviderType;
  auto_approve: boolean;
}

export interface SessionStatusInfo {
  sessionName: string;
  status: "active" | "idle" | "dead";
  providerType: ProviderType;
  unreadSince?: string | null;
}

export type ProviderType = "claude" | "codex" | "gemini" | "shell";

export const PROVIDER_OPTIONS: { value: ProviderType; label: string; description: string }[] = [
  { value: "claude", label: "Claude Code", description: "Anthropic's official CLI" },
  { value: "codex", label: "Codex", description: "OpenAI's CLI" },
  { value: "gemini", label: "Gemini CLI", description: "Google's AI CLI" },
  { value: "shell", label: "Terminal", description: "Plain shell terminal" },
];

export interface CreateSessionParams {
  name?: string;
  provider_type: ProviderType;
  source?: string;
  auto_approve?: boolean;
  profile?: string;
  branch?: string;
}

// File system types (matches internal/node/files/types.go)
export interface FileNode {
  name: string;
  path: string;
  type: "file" | "directory";
  size?: number;
  extension?: string;
  children?: FileNode[];
}

export interface FilesResponse {
  files: FileNode[];
  path: string; // expanded absolute path
}

// File metadata (matches GET /node/api/files/meta response)
export interface FileMetaResponse {
  size: number;
  isBinary: boolean;
  contentType: string;
  path: string;
}

// File search types (matches internal/node/filesearch/types.go)
export interface FileSearchResult {
  name: string;
  path: string;
  type: "file" | "directory";
}

export interface FileSearchResponse {
  results: FileSearchResult[];
  query: string;
  count: number;
}

// File upload types (matches POST /node/api/files/upload response)
export interface UploadedFile {
  path: string;
  name: string;
  size: number;
}

export interface UploadResponse {
  files: UploadedFile[];
}

// Git types (matches internal/node/git/types.go)
export type FileStatus =
  | "modified"
  | "added"
  | "deleted"
  | "renamed"
  | "copied"
  | "untracked"
  | "unmerged";

export interface GitFile {
  path: string;
  status: FileStatus;
  staged: boolean;
  oldPath?: string;
}

export interface GitStatus {
  branch: string;
  ahead: number;
  behind: number;
  staged: GitFile[];
  unstaged: GitFile[];
  untracked: GitFile[];
}

export interface CommitSummary {
  hash: string;
  shortHash: string;
  subject: string;
  body: string;
  author: string;
  authorEmail: string;
  timestamp: string;
  relativeTime: string;
  filesChanged: number;
  additions: number;
  deletions: number;
}

export interface CommitFile {
  path: string;
  status: FileStatus;
  additions: number;
  deletions: number;
  oldPath?: string;
}

export interface CommitDetail extends CommitSummary {
  files: CommitFile[];
}

export interface CompareResult {
  diff: string;
  files: CommitFile[];
  totalAdditions: number;
  totalDeletions: number;
  baseRef: string;
  headRef: string;
  totalLines: Record<string, number>;
}

export interface WorkingDiffResult {
  diff: string;
  files: CommitFile[];
  totalAdditions: number;
  totalDeletions: number;
  totalLines: Record<string, number>;
  fingerprint: string;
}

export interface CommitFullDiffResult {
  diff: string;
  totalLines: Record<string, number>;
}

export interface BranchList {
  branches: string[];
  defaultBase: string;
}

export type {
  DiffPosition,
  LineRange,
  ReviewComment,
  ReviewBody,
  Review,
} from "./types/review";
