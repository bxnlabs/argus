"use client";

import { useState, useCallback, useEffect, useRef } from "react";

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
 * Uses a counter-based approach to track nested dragenter/dragleave pairs,
 * which is more reliable than relatedTarget checking (relatedTarget can be
 * null with canvas elements like xterm, causing flicker).
 *
 * @param onFileDrop - Callback when files are dropped
 * @param options - Optional configuration
 * @returns isDragging state and drag event handlers to spread onto the container
 */
export function useFileDrop(
  onFileDrop: (files: File[]) => void,
  options?: UseFileDropOptions
): { isDragging: boolean; dragHandlers: DragHandlers } {
  const [isDragging, setIsDragging] = useState(false);
  const dragCountRef = useRef(0);

  // Reset drag state when disabled
  useEffect(() => {
    if (options?.disabled) {
      setIsDragging(false);
      dragCountRef.current = 0;
    }
  }, [options?.disabled]);

  // Reset on interrupted drags (file dragged out of window, tab switch, etc.)
  // The Terminal is long-lived so we can't rely on unmount to clean up.
  useEffect(() => {
    const reset = () => {
      if (dragCountRef.current > 0) {
        dragCountRef.current = 0;
        setIsDragging(false);
      }
    };
    document.addEventListener("drop", reset);
    document.addEventListener("dragend", reset);
    document.addEventListener("visibilitychange", reset);
    return () => {
      document.removeEventListener("drop", reset);
      document.removeEventListener("dragend", reset);
      document.removeEventListener("visibilitychange", reset);
    };
  }, []);

  const handleDragEnter = useCallback(
    (e: React.DragEvent) => {
      if (!e.dataTransfer?.types?.includes("Files")) return;
      e.preventDefault();
      e.stopPropagation();
      if (!options?.disabled) {
        dragCountRef.current++;
        if (dragCountRef.current === 1) {
          setIsDragging(true);
        }
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
      if (!e.dataTransfer?.types?.includes("Files")) return;
      e.preventDefault();
      e.stopPropagation();
      dragCountRef.current = Math.max(0, dragCountRef.current - 1);
      if (dragCountRef.current === 0) {
        setIsDragging(false);
      }
    },
    []
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      dragCountRef.current = 0;
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
