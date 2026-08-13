import { useCallback, useState } from "react";

export interface UseFileEditorReturn {
  openPaths: string[];
  activeFilePath: string | null;
  openFile: (path: string) => void;
  closeFile: (path: string) => void;
  setActiveFile: (path: string | null) => void;
}

/**
 * Which files the mobile explorer has open, and which one is showing.
 * Content is not here: it is a query, cached by path (see useFileContentQuery).
 */
export function useFileEditor(): UseFileEditorReturn {
  const [openPaths, setOpenPaths] = useState<string[]>([]);
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null);

  const openFile = useCallback((path: string) => {
    setOpenPaths((prev) => (prev.includes(path) ? prev : [...prev, path]));
    setActiveFilePath(path);
  }, []);

  const closeFile = useCallback((path: string) => {
    setOpenPaths((prev) => {
      const next = prev.filter((p) => p !== path);
      setActiveFilePath((current) => {
        if (current !== path) return current;
        // Fall back to the neighbour on the right (same index in the
        // filtered array); if the closed tab was last, Math.min pulls that
        // back to the new last element — the neighbour on the left. Empty
        // array falls back to null via the `?? null` below.
        const i = prev.indexOf(path);
        return next[Math.min(i, next.length - 1)] ?? null;
      });
      return next;
    });
  }, []);

  return { openPaths, activeFilePath, openFile, closeFile, setActiveFile: setActiveFilePath };
}
