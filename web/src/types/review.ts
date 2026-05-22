export type DiffSide = "L" | "R";

export interface DiffPosition {
  side: DiffSide;
  line: number;
}

export interface LineRange {
  from: DiffPosition;
  to: DiffPosition;
}

export type AnchorStatus = "stale" | "unanchored";

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
