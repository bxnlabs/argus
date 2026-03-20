export interface LineRange {
  from: number;
  to: number;
}

export interface InlineComment {
  id: string;
  file: string;
  line: LineRange;
  snippet: string;
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface GeneralComment {
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface CommentsFile {
  branch: string;
  baseBranch: string;
  comments: InlineComment[];
  generalComment?: GeneralComment;
}
