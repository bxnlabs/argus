import { useState, useCallback, useRef, useEffect } from "react";

/**
 * Defers mounting of heavy content until its container is near the viewport.
 * Once mounted, content stays mounted permanently (sticky) to preserve state.
 *
 * @param rootMargin - IntersectionObserver rootMargin. Default "200px" pre-mounts
 *                     content 200px before it scrolls into view.
 */
export function useLazyMount(rootMargin = "200px") {
  const [shouldMount, setShouldMount] = useState(false);
  const elementRef = useRef<HTMLElement | null>(null);
  const observerRef = useRef<IntersectionObserver | null>(null);
  const mountedRef = useRef(false);

  const ref = useCallback(
    (node: HTMLElement | null) => {
      // Cleanup previous observer
      if (observerRef.current) {
        observerRef.current.disconnect();
        observerRef.current = null;
      }

      elementRef.current = node;

      // Already mounted — no need to observe
      if (mountedRef.current || !node) return;

      const observer = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) {
              mountedRef.current = true;
              setShouldMount(true);
              observer.disconnect();
              observerRef.current = null;
              return;
            }
          }
        },
        { rootMargin },
      );

      observer.observe(node);
      observerRef.current = observer;
    },
    [rootMargin],
  );

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      observerRef.current?.disconnect();
    };
  }, []);

  return { ref, shouldMount };
}
