import { useState, useRef, useCallback, useMemo, useEffect } from "react";
import {
  Loader2,
  GitCompareArrows,
  FilePlus,
  FileX,
  FileText,
  ArrowRight,
  ArrowLeft,
  AlertCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { UnifiedDiff } from "@/components/DiffViewer/UnifiedDiff";
import { parseMultiFileDiff, getDiffFileName, getDiffPathKey } from "@/lib/diff-parser";
import { useCompareBranchesQuery, useCompareQuery } from "@/data/git";
import { useViewport } from "@/hooks/useViewport";
import type { CommitFile, FileStatus } from "@/types";

interface CompareViewProps {
  workingDirectory: string;
  currentBranch?: string;
  header?: React.ReactNode;
  listWidth?: number;
  onResizeMouseDown?: (e: React.MouseEvent) => void;
}

export function CompareView({ workingDirectory, currentBranch, header, listWidth, onResizeMouseDown }: CompareViewProps) {
  const { isMobile } = useViewport();
  const [baseBranch, setBaseBranch] = useState<string | null>(null);
  const [mobileShowDiffs, setMobileShowDiffs] = useState(false);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);

  const {
    data: branchData,
    isLoading: loadingBranches,
    isError: branchError,
    error: branchErrorDetail,
  } = useCompareBranchesQuery(workingDirectory);

  // Reset repo-scoped state when working directory changes
  useEffect(() => {
    setBaseBranch(null);
    setSelectedPath(null);
    diffRefs.current.clear();
  }, [workingDirectory]);

  // Branches excluding the current one (comparing with yourself is useless)
  const availableBranches = useMemo(() => {
    if (!branchData?.branches) return [];
    if (!currentBranch) return branchData.branches;
    return branchData.branches.filter((b) => b !== currentBranch);
  }, [branchData?.branches, currentBranch]);

  // Set default base branch when branch data loads
  useEffect(() => {
    if (baseBranch !== null) return;
    if (!branchData) return;
    const defaultBase =
      branchData.defaultBase && branchData.defaultBase !== currentBranch
        ? branchData.defaultBase
        : null;
    setBaseBranch(defaultBase || availableBranches[0] || null);
  }, [branchData, baseBranch, currentBranch, availableBranches]);

  const {
    data: compareData,
    isLoading: loadingCompare,
    isError: compareError,
    error: compareErrorDetail,
  } = useCompareQuery(workingDirectory, baseBranch);

  const parsedDiffs = useMemo(() => {
    if (!compareData?.diff) return [];
    return parseMultiFileDiff(compareData.diff);
  }, [compareData?.diff]);

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

  const scrollToFile = useCallback((path: string) => {
    setSelectedPath(path);
    if (isMobile) {
      setMobileShowDiffs(true);
    } else {
      const el = diffRefs.current.get(path);
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  }, [isMobile]);

  // Scroll to selected file once diffs are rendered and refs are ready
  useEffect(() => {
    if (!selectedPath || !mobileShowDiffs) return;
    const el = diffRefs.current.get(selectedPath);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [selectedPath, parsedDiffs, mobileShowDiffs]);

  if (loadingBranches) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
      </div>
    );
  }

  if (branchError) {
    return (
      <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
        <AlertCircle className="mb-2 h-8 w-8 opacity-50" />
        <p className="text-sm">
          {branchErrorDetail instanceof Error
            ? branchErrorDetail.message
            : "Failed to load branches"}
        </p>
      </div>
    );
  }

  if (!baseBranch && availableBranches.length === 0) {
    return (
      <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
        <GitCompareArrows className="mb-2 h-8 w-8 opacity-50" />
        <p className="text-sm">No branches available to compare</p>
        <p className="text-xs">
          Create another branch or set an upstream tracking branch
        </p>
      </div>
    );
  }

  const branchSelector = (
    <div className="flex items-center gap-2 px-3 py-2">
      <span className="text-muted-foreground text-xs">Base:</span>
      <select
        value={baseBranch ?? ""}
        onChange={(e) => setBaseBranch(e.target.value)}
        className="bg-muted border-border rounded border px-2 py-1 text-xs"
      >
        {availableBranches.map((branch) => (
          <option key={branch} value={branch}>
            {branch}
          </option>
        ))}
      </select>
    </div>
  );

  const summary = compareData ? (
    <div className="text-muted-foreground border-border/50 border-b px-3 py-1.5 text-xs">
      {compareData.files.length} file{compareData.files.length !== 1 ? "s" : ""} changed
      {compareData.totalAdditions > 0 && (
        <span className="ml-2 text-green-500">+{compareData.totalAdditions}</span>
      )}
      {compareData.totalDeletions > 0 && (
        <span className="ml-1 text-red-500">-{compareData.totalDeletions}</span>
      )}
    </div>
  ) : null;

  const fileList = compareData?.files.length ? (
    <div className="flex-1 overflow-y-auto">
      {compareData.files.map((file) => (
        <CompareFileRow
          key={file.path}
          file={file}
          isSelected={selectedPath === file.path}
          onClick={() => scrollToFile(file.path)}
        />
      ))}
    </div>
  ) : null;

  const diffPane = (
    <div className="flex-1 overflow-y-auto p-3">
      {loadingCompare ? (
        <div className="flex h-32 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      ) : compareError ? (
        <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
          <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
          <p className="text-sm">
            {compareErrorDetail instanceof Error
              ? compareErrorDetail.message
              : "Failed to compare branches"}
          </p>
        </div>
      ) : parsedDiffs.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
          <GitCompareArrows className="mb-4 h-12 w-12 opacity-50" />
          <p className="text-sm">No changes between branches</p>
        </div>
      ) : (
        <div className="space-y-3">
          {parsedDiffs.map((diff) => {
            const pathKey = getDiffPathKey(diff);
            const fileName = getDiffFileName(diff);
            return (
              <div key={pathKey} ref={setDiffRef(pathKey)}>
                <UnifiedDiff diff={diff} fileName={fileName} expanded />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );

  // Mobile: full-screen diff view when user taps a file
  if (isMobile && mobileShowDiffs) {
    return (
      <div className="flex h-full flex-col">
        <div className="bg-muted/30 flex items-center gap-2 p-2">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setMobileShowDiffs(false)}
            aria-label="Back to file list"
          >
            <ArrowLeft className="h-5 w-5" />
          </Button>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">
              {baseBranch ? `Changes from ${baseBranch}` : "Compare"}
            </p>
            {compareData && (
              <p className="text-muted-foreground text-xs">
                {compareData.files.length} file{compareData.files.length !== 1 ? "s" : ""} changed
              </p>
            )}
          </div>
        </div>
        <div className="safe-area-bottom flex-1 overflow-auto">
          {loadingCompare ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : compareError ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
              <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">
                {compareErrorDetail instanceof Error
                  ? compareErrorDetail.message
                  : "Failed to compare branches"}
              </p>
            </div>
          ) : (
            <div className="space-y-3 p-3">
              {parsedDiffs.map((diff) => {
                const pathKey = getDiffPathKey(diff);
                const fileName = getDiffFileName(diff);
                return (
                  <div key={pathKey} ref={setDiffRef(pathKey)}>
                    <UnifiedDiff diff={diff} fileName={fileName} expanded />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    );
  }

  // Mobile: file list view (default)
  if (isMobile) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        {branchSelector}
        {summary}
        <div className="safe-area-bottom flex-1 overflow-y-auto">
          {loadingCompare ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : compareError ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
              <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">
                {compareErrorDetail instanceof Error
                  ? compareErrorDetail.message
                  : "Failed to compare branches"}
              </p>
            </div>
          ) : compareData?.files.length ? (
            compareData.files.map((file) => (
              <CompareFileRow
                key={file.path}
                file={file}
                isSelected={selectedPath === file.path}
                onClick={() => scrollToFile(file.path)}
              />
            ))
          ) : compareData ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
              <GitCompareArrows className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">No changes between branches</p>
            </div>
          ) : null}
        </div>
      </div>
    );
  }

  // Desktop layout
  return (
    <div className="flex min-h-0 flex-1">
      {/* Left sidebar */}
      <div className="flex h-full flex-col" style={{ width: listWidth }}>
        {header}
        {branchSelector}
        {summary}
        {fileList}
      </div>

      {/* Resize handle */}
      <div
        className="bg-muted/50 hover:bg-primary/50 active:bg-primary w-1 flex-shrink-0 cursor-col-resize transition-colors"
        onMouseDown={onResizeMouseDown}
      />

      {/* Right pane */}
      <div className="bg-muted/20 flex min-w-0 flex-1 flex-col">
        {diffPane}
      </div>
    </div>
  );
}

function CompareFileRow({
  file,
  isSelected,
  onClick,
}: {
  file: CommitFile;
  isSelected: boolean;
  onClick: () => void;
}) {
  const StatusIcon = getStatusIcon(file.status);

  return (
    <button
      onClick={onClick}
      className={cn(
        "hover:bg-muted/70 flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors",
        isSelected && "bg-primary/10 hover:bg-primary/20",
      )}
    >
      <StatusIcon
        className={cn("h-4 w-4 flex-shrink-0", getStatusColor(file.status))}
      />
      <span className="flex-1 truncate text-sm">
        {file.oldPath ? (
          <span className="flex items-center gap-1">
            <span className="text-muted-foreground">{file.oldPath}</span>
            <ArrowRight className="h-3 w-3" />
            <span>{file.path}</span>
          </span>
        ) : (
          file.path
        )}
      </span>
      <div className="flex flex-shrink-0 items-center gap-1 text-xs">
        {file.additions > 0 && (
          <span className="text-green-500">+{file.additions}</span>
        )}
        {file.deletions > 0 && (
          <span className="text-red-500">-{file.deletions}</span>
        )}
      </div>
    </button>
  );
}

function getStatusIcon(status: FileStatus) {
  switch (status) {
    case "added":
      return FilePlus;
    case "deleted":
      return FileX;
    case "renamed":
      return ArrowRight;
    default:
      return FileText;
  }
}

function getStatusColor(status: FileStatus): string {
  switch (status) {
    case "added":
      return "text-green-500";
    case "deleted":
      return "text-red-500";
    case "renamed":
      return "text-yellow-500";
    default:
      return "text-muted-foreground";
  }
}
