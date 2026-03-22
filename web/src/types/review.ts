export interface LineRange {
  from: number;
  to: number;
}

export interface ReviewComment {
  id: string;
  file: string;
  line: LineRange;
  snippet: string;
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface Review {
  head: string;
  base: string;
  body?: string;
  comments: ReviewComment[];
}
