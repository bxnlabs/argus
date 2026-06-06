import { useState, useCallback, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch, apiTextFetch } from "@/api/client";
import { filesKeys } from "@/data/files/keys";
import type { FileMetaResponse } from "@/types";

export interface OpenFile {
  path: string;
  content: string;
  originalContent: string;
  language: string;
  isBinary: boolean;
  isLarge: boolean;
}

export interface UseFileEditorReturn {
  openFiles: OpenFile[];
  activeFilePath: string | null;
  loading: boolean;
  saving: boolean;
  openFile: (path: string) => Promise<void>;
  closeFile: (path: string) => void;
  setActiveFile: (path: string | null) => void;
  updateContent: (path: string, content: string) => void;
  saveFile: (path: string) => Promise<boolean>;
  saveAllDirty: () => Promise<void>;
  isDirty: (path: string) => boolean;
  hasDirtyFiles: () => boolean;
  getFile: (path: string) => OpenFile | undefined;
}

const LARGE_FILE_THRESHOLD = 5 * 1024 * 1024; // 5MB

function getLanguageFromPath(filePath: string): string {
  const ext = filePath.split(".").pop()?.toLowerCase() || "";
  const map: Record<string, string> = {
    ts: "typescript",
    tsx: "typescript",
    js: "javascript",
    jsx: "javascript",
    json: "json",
    md: "markdown",
    html: "html",
    htm: "html",
    css: "css",
    scss: "scss",
    less: "less",
    xml: "xml",
    yaml: "yaml",
    yml: "yaml",
    py: "python",
    go: "go",
    rs: "rust",
    sh: "shell",
    bash: "shell",
    zsh: "shell",
    sql: "sql",
    graphql: "graphql",
    dockerfile: "dockerfile",
    toml: "toml",
    ini: "ini",
    c: "c",
    cpp: "cpp",
    h: "c",
    hpp: "cpp",
    java: "java",
    rb: "ruby",
    php: "php",
    swift: "swift",
    kt: "kotlin",
    lua: "lua",
    r: "r",
  };
  return map[ext] || "plaintext";
}

export function useFileEditor(): UseFileEditorReturn {
  const queryClient = useQueryClient();
  const [openFiles, setOpenFiles] = useState<OpenFile[]>([]);
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  // Use ref to access current openFiles in callbacks without stale closures
  const openFilesRef = useRef(openFiles);
  openFilesRef.current = openFiles;
  // Track in-flight file opens to prevent duplicate tabs from concurrent calls
  const openingPathsRef = useRef<Set<string>>(new Set());

  const getFile = useCallback(
    (path: string) => openFilesRef.current.find((f) => f.path === path),
    [],
  );

  const isDirty = useCallback(
    (path: string) => {
      const file = openFilesRef.current.find((f) => f.path === path);
      return file ? file.content !== file.originalContent : false;
    },
    [],
  );

  const hasDirtyFiles = useCallback(
    () => openFilesRef.current.some((f) => f.content !== f.originalContent),
    [],
  );

  const openFile = useCallback(async (path: string) => {
    // Focus existing tab
    const existing = openFilesRef.current.find((f) => f.path === path);
    if (existing) {
      setActiveFilePath(path);
      return;
    }

    // Prevent concurrent opens of the same file (e.g., double-click)
    if (openingPathsRef.current.has(path)) return;
    openingPathsRef.current.add(path);

    setLoading(true);
    try {
      // 1. Fetch metadata
      const meta = await apiFetch<FileMetaResponse>(
        `/api/node/files/meta?path=${encodeURIComponent(path)}`,
      );

      const isLarge = meta.size > LARGE_FILE_THRESHOLD;
      const isBinary = meta.isBinary;

      let content = "";
      if (!isBinary && !isLarge) {
        // 2. Fetch content as raw text
        const res = await apiTextFetch(
          `/api/node/files/content?path=${encodeURIComponent(path)}`,
        );
        content = await res.text();
      }

      const newFile: OpenFile = {
        path,
        content,
        originalContent: content,
        language: getLanguageFromPath(path),
        isBinary,
        isLarge,
      };

      setOpenFiles((prev) => [...prev, newFile]);
      setActiveFilePath(path);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to open file",
      );
    } finally {
      openingPathsRef.current.delete(path);
      setLoading(false);
    }
  }, []);

  const closeFile = useCallback((path: string) => {
    setOpenFiles((prev) => {
      const newFiles = prev.filter((f) => f.path !== path);
      setActiveFilePath((current) => {
        if (current !== path) return current;
        if (newFiles.length === 0) return null;
        const closedIndex = prev.findIndex((f) => f.path === path);
        if (closedIndex >= newFiles.length)
          return newFiles[newFiles.length - 1].path;
        return newFiles[closedIndex].path;
      });
      return newFiles;
    });
  }, []);

  const updateContent = useCallback((path: string, content: string) => {
    setOpenFiles((prev) =>
      prev.map((f) => (f.path === path ? { ...f, content } : f)),
    );
  }, []);

  const saveFile = useCallback(async (path: string): Promise<boolean> => {
    const file = openFilesRef.current.find((f) => f.path === path);
    if (!file || file.isBinary) return false;

    // Capture content at call time to avoid race conditions
    // (user may edit further while save is in flight)
    const contentToSave = file.content;

    setSaving(true);
    try {
      await apiTextFetch(
        `/api/node/files/content?path=${encodeURIComponent(path)}`,
        {
          method: "PUT",
          headers: { "Content-Type": "text/plain" },
          body: contentToSave,
        },
      );

      const fileName = path.split("/").pop() || path;
      toast.success(`Saved ${fileName}`);

      // Update originalContent to the content we actually saved
      // (not current content, which may have changed during save)
      setOpenFiles((prev) =>
        prev.map((f) =>
          f.path === path ? { ...f, originalContent: contentToSave } : f,
        ),
      );

      // Invalidate parent directory listing so file sizes update
      const parentDir = path.substring(0, path.lastIndexOf("/"));
      if (parentDir) {
        queryClient.invalidateQueries({ queryKey: filesKeys.list(parentDir) });
      }

      return true;
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to save file",
      );
      return false;
    } finally {
      setSaving(false);
    }
  }, [queryClient]);

  const saveAllDirty = useCallback(async () => {
    const dirtyFiles = openFilesRef.current.filter(
      (f) => f.content !== f.originalContent && !f.isBinary,
    );
    for (const file of dirtyFiles) {
      await saveFile(file.path);
    }
  }, [saveFile]);

  return {
    openFiles,
    activeFilePath,
    loading,
    saving,
    openFile,
    closeFile,
    setActiveFile: setActiveFilePath,
    updateContent,
    saveFile,
    saveAllDirty,
    isDirty,
    hasDirtyFiles,
    getFile,
  };
}
