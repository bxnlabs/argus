/** Touch-primary device — stable across orientation changes */
export const isTouchDevice =
  typeof window !== "undefined" &&
  window.matchMedia("(pointer: coarse)").matches;

/** Phone-sized: shortest screen dimension is under 768px */
export function isPhoneSized() {
  return Math.min(window.innerWidth, window.innerHeight) < 768;
}

/** macOS / iOS device — checks userAgent at call time for testability */
export function isMac() {
  return (
    typeof navigator !== "undefined" &&
    /Mac|iPhone|iPad/.test(navigator.userAgent)
  );
}
