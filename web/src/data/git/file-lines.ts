import { apiFetch } from "@/api/client";

export interface FileLinesParams {
  path: string;
  file: string;
  start: number;
  end: number;
  ref?: string;
  signal?: AbortSignal;
}

export interface FileLinesResult {
  lines: string[];
  start: number;
  end: number;
  totalLines: number;
}

export async function fetchFileLines(params: FileLinesParams): Promise<FileLinesResult> {
  const searchParams = new URLSearchParams({
    path: params.path,
    file: params.file,
    start: String(params.start),
    end: String(params.end),
  });
  if (params.ref) {
    searchParams.set("ref", params.ref);
  }

  return apiFetch<FileLinesResult>(
    `/node/api/git/file-lines?${searchParams.toString()}`,
    { signal: params.signal },
  );
}
