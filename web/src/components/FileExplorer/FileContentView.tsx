import { Suspense, lazy } from "react";
import { AlertCircle, AlertTriangle, Loader2 } from "lucide-react";
import { EditorSkeleton } from "./EditorSkeleton";
import { isDefinitiveReadError, type FileContent } from "@/data/files";

const FileEditor = lazy(() =>
  import("./FileEditor").then((mod) => ({ default: mod.FileEditor })),
);

export interface FileContentViewProps {
  /** The content query's result, passed in so each caller keeps its own reload control. */
  data: FileContent | undefined;
  isPending: boolean;
  isError: boolean;
  error: Error | null | undefined;
}

/**
 * One open file's body: the pending, failed, and loaded states of a content
 * query, and the editor itself.
 *
 * Shared by EditorCenter (desktop dock pane) and FileExplorer (mobile drill-in)
 * so the stale-vs-definitive rule below is decided in one place. Each caller
 * still owns its own chrome above this — a path bar on desktop, a back button
 * and tab strip on mobile — and its own reload control, which is why the query
 * result comes in as props rather than being read here.
 */
export function FileContentView({ data, isPending, isError, error }: FileContentViewProps) {
  if (isPending) {
    return (
      <div data-testid="file-loading" className="flex h-full items-center justify-center">
        <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
      </div>
    );
  }

  // Stale content wins over a blip, but not over a verdict. A refetch that
  // fails transiently (the node is briefly unreachable on window focus) must
  // leave an already-open file on screen rather than blanking it — but a 4xx
  // is the node answering: the file is gone, or no longer readable. Holding
  // its bytes on screen after that would show content for a path that does
  // not exist, indefinitely, with nothing to say so.
  if (isError && (!data || isDefinitiveReadError(error))) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-4">
        <AlertCircle className="text-muted-foreground mb-2 h-8 w-8" />
        <p className="text-muted-foreground text-center text-sm">
          {error instanceof Error ? error.message : "Failed to open file"}
        </p>
      </div>
    );
  }

  // Unreachable: isPending and the error branch above already cover every
  // status that lacks data. Keeps TS happy without an assertion below.
  if (!data) return null;

  const editor = (
    <Suspense fallback={<EditorSkeleton />}>
      <FileEditor
        content={data.content}
        language={data.language}
        isBinary={data.isBinary}
        isLarge={data.isLarge}
      />
    </Suspense>
  );

  // Holding stale bytes through a blip is the right trade (above), but only
  // while the user can tell that is what they are looking at. A route stuck on
  // a 5xx fails every poll and every reload in silence, and a pane frozen on
  // a version from an hour ago is indistinguishable from one that is current —
  // which is the exact confusion this whole polled read exists to prevent.
  if (!isError) return editor;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div
        role="status"
        className="flex flex-shrink-0 items-center gap-1.5 border-b border-amber-500/40 bg-amber-500/10 px-3 py-1 text-[11px] text-amber-700 dark:text-amber-400"
      >
        <AlertTriangle className="h-3 w-3 flex-shrink-0" />
        <span className="min-w-0 truncate">
          Showing the last version read — refresh failed
          {error instanceof Error ? `: ${error.message}` : ""}
        </span>
      </div>
      <div className="min-h-0 flex-1">{editor}</div>
    </div>
  );
}
