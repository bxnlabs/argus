import { useState, useCallback, useRef, useMemo } from "react";
import { Loader2, History, ArrowLeft, FileCode } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CommitItem } from "./CommitItem";
import { UnifiedDiff } from "@/components/DiffViewer/UnifiedDiff";
import {
  useGitHistoryQuery,
  useCommitFullDiffQuery,
} from "@/data/git";
import { parseMultiFileDiff, getDiffFileName } from "@/lib/diff-parser";
import { useViewport } from "@/hooks/useViewport";
import type { CommitFile } from "@/types";

interface CommitHistoryProps {
  workingDirectory: string;
  header?: React.ReactNode;
}

export function CommitHistory({ workingDirectory, header }: CommitHistoryProps) {
  const { isMobile } = useViewport();
  const {
    data: commits,
    isLoading,
    error,
  } = useGitHistoryQuery(workingDirectory);

  const [expandedHash, setExpandedHash] = useState<string | null>(null);
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  // Track if user tapped a file on mobile to navigate to full-screen diff view
  const [mobileShowDiffs, setMobileShowDiffs] = useState(false);

  const { data: fullDiff, isLoading: loadingDiff } = useCommitFullDiffQuery(
    workingDirectory,
    expandedHash,
  );

  const parsedDiffs = useMemo(() => {
    if (!fullDiff) return [];
    return parseMultiFileDiff(fullDiff);
  }, [fullDiff]);

  const setDiffRef = useCallback(
    (path: string) => (el: HTMLDivElement | null) => {
      if (el) {
        diffRefs.current.set(path, el);
      } else {
        diffRefs.current.delete(path);
      }
    },
    [],
  );

  const handleToggleCommit = useCallback((hash: string) => {
    setExpandedHash((prev) => {
      if (prev === hash) {
        return null;
      }
      return hash;
    });
    setSelectedFilePath(null);
    setMobileShowDiffs(false);
  }, []);

  const handleFileClick = useCallback(
    (hash: string, file: CommitFile) => {
      setExpandedHash(hash);
      setSelectedFilePath(file.path);

      if (isMobile) {
        setMobileShowDiffs(true);
        // Scroll after render
        requestAnimationFrame(() => {
          const el = diffRefs.current.get(file.path);
          if (el) {
            el.scrollIntoView({ behavior: "smooth", block: "start" });
          }
        });
      } else {
        // Desktop: scroll in the right pane
        requestAnimationFrame(() => {
          const el = diffRefs.current.get(file.path);
          if (el) {
            el.scrollIntoView({ behavior: "smooth", block: "start" });
          }
        });
      }
    },
    [isMobile],
  );

  const stackedDiffs = (
    <div className="space-y-3 p-3">
      {parsedDiffs.map((diff) => {
        const fileName = getDiffFileName(diff);
        return (
          <div key={fileName} ref={setDiffRef(fileName)}>
            <UnifiedDiff diff={diff} fileName={fileName} expanded />
          </div>
        );
      })}
    </div>
  );

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
          <History className="mb-2 h-8 w-8 opacity-50" />
          <p className="text-center text-sm">Failed to load commit history</p>
        </div>
      </div>
    );
  }

  if (!commits?.length) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
          <History className="mb-2 h-8 w-8 opacity-50" />
          <p className="text-sm">No commits yet</p>
        </div>
      </div>
    );
  }

  // Mobile: full-screen stacked diff view when user taps a file
  if (isMobile && mobileShowDiffs && expandedHash) {
    const commit = commits.find((c) => c.hash === expandedHash);
    return (
      <div className="flex h-full flex-col">
        <div className="bg-muted/30 flex items-center gap-2 p-2">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setMobileShowDiffs(false)}
          >
            <ArrowLeft className="h-5 w-5" />
          </Button>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">
              {commit?.subject ?? expandedHash.slice(0, 7)}
            </p>
            <p className="text-muted-foreground text-xs">
              {expandedHash.slice(0, 7)}
            </p>
          </div>
        </div>
        <div className="flex-1 overflow-auto">
          {loadingDiff ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : (
            stackedDiffs
          )}
        </div>
      </div>
    );
  }

  // Mobile: commit list only
  if (isMobile) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="flex-1 overflow-y-auto">
          {commits.map((commit) => (
            <CommitItem
              key={commit.hash}
              commit={commit}
              workingDir={workingDirectory}
              expanded={expandedHash === commit.hash}
              onToggle={() => handleToggleCommit(commit.hash)}
              onFileClick={handleFileClick}
              selectedFile={
                selectedFilePath && expandedHash
                  ? { hash: expandedHash, path: selectedFilePath }
                  : null
              }
            />
          ))}
        </div>
      </div>
    );
  }

  // Desktop: side-by-side layout
  return (
    <div className="flex min-h-0 flex-1">
      {/* Commit list */}
      <div className="flex w-[300px] flex-shrink-0 flex-col">
        {header}
        <div className="flex-1 overflow-y-auto">
          {commits.map((commit) => (
            <CommitItem
              key={commit.hash}
              commit={commit}
              workingDir={workingDirectory}
              expanded={expandedHash === commit.hash}
              onToggle={() => handleToggleCommit(commit.hash)}
              onFileClick={handleFileClick}
              selectedFile={
                selectedFilePath && expandedHash
                  ? { hash: expandedHash, path: selectedFilePath }
                  : null
              }
            />
          ))}
        </div>
      </div>

      {/* Divider */}
      <div className="bg-muted/50 w-1 flex-shrink-0" />

      {/* Diff view - stacked diffs */}
      <div className="bg-muted/20 flex min-w-0 flex-1 flex-col">
        {loadingDiff ? (
          <div className="flex flex-1 items-center justify-center">
            <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
          </div>
        ) : expandedHash && parsedDiffs.length > 0 ? (
          <div className="flex-1 overflow-auto">{stackedDiffs}</div>
        ) : (
          <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center">
            <FileCode className="mb-4 h-12 w-12 opacity-50" />
            <p className="text-sm">Select a commit to view changes</p>
          </div>
        )}
      </div>
    </div>
  );
}
