export type DiffSide = "L" | "R";

export interface DiffPosition {
  side: DiffSide;
  line: number;
}

export interface LineRange {
  from: DiffPosition;
  to: DiffPosition;
}

export type AnchorStatus = "resolved" | "stale" | "context_unavailable";

export interface ReviewComment {
  id: string;
  file: string;
  oldPath?: string;
  line: LineRange;
  snippet: string;
  snippetContext?: string;
  anchorStatus?: AnchorStatus;
  body: string;
  submitted: boolean;
  createdAt: string;

  /** Populated by the server on GET; never sent on POST (the server strips
   * them anyway, but client code should not read these fields to decide
   * what to persist). */
  orphaned?: boolean;
  orphanLine?: number;
  orphanSide?: DiffSide;
  orphanDeleted?: boolean;
}

export interface ReviewBody {
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface Review {
  head: string;
  base: string;
  body?: ReviewBody;
  comments: ReviewComment[];
}
