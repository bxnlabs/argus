"use client";

import { useState, useCallback, useEffect, type RefObject } from "react";

interface UseFileDropOptions {
  /** Disable drop handling (e.g., while uploading) */
  disabled?: boolean;
}

interface DragHandlers {
  onDragEnter: (e: React.DragEvent) => void;
  onDragOver: (e: React.DragEvent) => void;
  onDragLeave: (e: React.DragEvent) => void;
  onDrop: (e: React.DragEvent) => void;
}

/**
 * Hook for handling file drag and drop on a container element.
 *
 * @param containerRef - Ref to the container element for relatedTarget checking
 * @param onFileDrop - Callback when files are dropped
 * @param options - Optional configuration
 * @returns isDragging state and drag event handlers to spread onto the container
 */
export function useFileDrop(
  containerRef: RefObject<HTMLElement | null>,
  onFileDrop: (files: File[]) => void,
  options?: UseFileDropOptions
): { isDragging: boolean; dragHandlers: DragHandlers } {
  const [isDragging, setIsDragging] = useState(false);

  // Reset drag state when disabled
  useEffect(() => {
    if (options?.disabled) {
      setIsDragging(false);
    }
  }, [options?.disabled]);

  const handleDragEnter = useCallback(
    (e: React.DragEvent) => {
      if (!e.dataTransfer?.types?.includes("Files")) return;
      e.preventDefault();
      e.stopPropagation();
      if (!options?.disabled) {
        setIsDragging(true);
      }
    },
    [options?.disabled]
  );

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      if (!e.dataTransfer?.types?.includes("Files")) return;
      e.preventDefault();
      e.stopPropagation();
    },
    []
  );

  const handleDragLeave = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      // Only set to false if leaving the container entirely
      // This prevents flickering when moving over nested elements
      if (!containerRef.current?.contains(e.relatedTarget as Node)) {
        setIsDragging(false);
      }
    },
    [containerRef]
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);

      if (options?.disabled || !e.dataTransfer?.types?.includes("Files")) return;

      const files = Array.from(e.dataTransfer.files);
      if (files.length > 0) {
        onFileDrop(files);
      }
    },
    [onFileDrop, options?.disabled]
  );

  return {
    isDragging,
    dragHandlers: {
      onDragEnter: handleDragEnter,
      onDragOver: handleDragOver,
      onDragLeave: handleDragLeave,
      onDrop: handleDrop,
    },
  };
}
