export interface Session {
  id: string;
  name: string;
  /** Server-derived from `name`; see internal/slug. Safe for display as a channel. */
  slug: string;
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
  profile: string | null;
  pinned: boolean;
}

export interface SessionStatusInfo {
  sessionName: string;
  status: "active" | "idle" | "dead";
  providerType: ProviderType;
  unreadSince?: string | null;
  userMarkedUnreadAt?: string | null;
}

export type ProviderType = "claude" | "codex" | "shell";

export const PROVIDER_OPTIONS: { value: ProviderType; label: string; description: string }[] = [
  { value: "claude", label: "Claude Code", description: "Anthropic's official CLI" },
  { value: "codex", label: "Codex", description: "OpenAI's CLI" },
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

// One file as the viewer needs it (matches GET /api/node/files/content
// response).
//
// A union, because the node really does send two different objects and the
// narrow one omits every content field. Declaring them all required described
// only the full response and quietly promised `content` and `size` on a reply
// that carries neither; `unchanged` is the discriminant that makes reading them
// a compile error rather than a runtime `undefined`.
export type FileViewResponse = FileViewUnchanged | FileViewContent;

/**
 * The node's answer to a `known` etag that still matches: no bytes, because the
 * caller already has them. Only the validator and the path come back.
 */
export interface FileViewUnchanged {
  unchanged: true;
  etag: string;
  path: string;
}

/**
 * A full read. Content is empty whenever isBinary or isLarge is set — the node
 * classifies and reads from a single open descriptor, so these fields always
 * describe the same version of the file. Both flags can be set at once (an
 * oversized binary); isBinary is the stronger verdict for rendering.
 *
 * `size` is the size of the version `etag` names, and is deliberately not the
 * authority on `isLarge` — see ReadForViewer.
 */
export interface FileViewContent {
  unchanged?: false;
  content: string;
  size: number;
  isBinary: boolean;
  isLarge: boolean;
  etag: string;
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

// File upload types (matches POST /api/node/files/upload response)
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
  totalLines: Record<string, number>;
  totalAdditions: number;
  totalDeletions: number;
  baseRef: string;
  headRef: string;
  baseUpstream: string;
  baseBehindBy: number;
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
  DiffSide,
  DiffPosition,
  LineRange,
  AnchorStatus,
  ReviewComment,
  ReviewBody,
  Review,
} from "./types/review";

export type NodeSource = "local" | "manual" | "discovered";

export interface NodeInfo {
  id: string;
  name: string;
  url: string; // "" == same-origin (the local node)
  source: NodeSource;
  self: boolean;
}

export interface NodeSummary {
  attention: number;
  busy: number;
  total: number;
}

export interface NodeWithStatus extends NodeInfo {
  summary: NodeSummary | null;
  online: boolean;
  pending: boolean;
}
