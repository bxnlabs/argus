import { useState } from "react";
import {
  FileText,
  FilePlus,
  FileX,
  ArrowRight,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { GitFile, FileStatus } from "@/types";

interface FileChangesProps {
  files: GitFile[];
  title: string;
  selectedPath?: string;
  onFileClick: (file: GitFile) => void;
}

export function FileChanges({
  files,
  title,
  selectedPath,
  onFileClick,
}: FileChangesProps) {
  const [expanded, setExpanded] = useState(true);

  if (files.length === 0) return null;

  return (
    <div className="mb-4">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-muted-foreground hover:text-foreground flex items-center gap-2 text-sm font-medium transition-colors"
        >
          <ChevronRight
            className={cn(
              "h-4 w-4 transition-transform",
              expanded && "rotate-90",
            )}
          />
          <span>{title}</span>
        </button>
        <span className="bg-muted ml-auto rounded-full px-2 py-0.5 text-xs">
          {files.length}
        </span>
      </div>

      {expanded && (
        <div className="space-y-0.5">
          {files.map((file) => (
            <FileItem
              key={file.path}
              file={file}
              isSelected={file.path === selectedPath}
              onClick={() => onFileClick(file)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface FileItemProps {
  file: GitFile;
  isSelected: boolean;
  onClick: () => void;
}

function FileItem({ file, isSelected, onClick }: FileItemProps) {
  const StatusIcon = getStatusIcon(file.status);

  return (
    <button
      onClick={onClick}
      className={cn(
        "hover:bg-muted/70 flex min-h-[44px] w-full items-center gap-2 px-3 py-1.5 text-left transition-colors",
        isSelected && "bg-primary/10 hover:bg-primary/20",
      )}
    >
      <StatusIcon
        className={cn("h-4 w-4 flex-shrink-0", getStatusColor(file.status))}
      />
      <span className="min-w-0 flex-1 text-sm">
        {file.oldPath ? (
          <span className="flex min-w-0 items-center gap-1">
            <span className="text-muted-foreground truncate">{file.oldPath}</span>
            <ArrowRight className="h-3 w-3 flex-shrink-0" />
            <span className="truncate">{file.path}</span>
          </span>
        ) : (
          <span className="block truncate">{file.path}</span>
        )}
      </span>
    </button>
  );
}

function getStatusIcon(status: FileStatus) {
  switch (status) {
    case "added":
    case "untracked":
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
    case "modified":
      return "text-yellow-500";
    case "added":
    case "untracked":
      return "text-green-500";
    case "deleted":
      return "text-red-500";
    case "renamed":
      return "text-blue-500";
    default:
      return "text-muted-foreground";
  }
}
