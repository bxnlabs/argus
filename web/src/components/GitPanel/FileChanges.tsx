import { useState } from "react";
import {
  File,
  Edit3,
  Plus,
  Minus,
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
  const statusIcon = getStatusIcon(file.status);
  const statusColor = getStatusColor(file.status);
  const fileName = file.path.split("/").pop() || file.path;
  const filePath = file.path.includes("/")
    ? file.path.slice(0, file.path.lastIndexOf("/"))
    : "";

  return (
    <button
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-2 text-sm transition-colors",
        "min-h-[44px]",
        isSelected ? "bg-accent" : "hover:bg-accent/50",
      )}
    >
      <span className={cn("flex-shrink-0", statusColor)}>{statusIcon}</span>
      <div className="min-w-0 flex-1 text-left">
        <span className="block truncate">{fileName}</span>
        {filePath && (
          <span className="text-muted-foreground block truncate text-xs">
            {filePath}
          </span>
        )}
      </div>
      <ArrowRight className="text-muted-foreground h-4 w-4 flex-shrink-0" />
    </button>
  );
}

function getStatusIcon(status: FileStatus) {
  switch (status) {
    case "modified":
      return <Edit3 className="h-4 w-4" />;
    case "added":
    case "untracked":
      return <Plus className="h-4 w-4" />;
    case "deleted":
      return <Minus className="h-4 w-4" />;
    case "renamed":
      return <ArrowRight className="h-4 w-4" />;
    default:
      return <File className="h-4 w-4" />;
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
