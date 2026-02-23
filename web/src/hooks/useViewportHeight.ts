import { useEffect } from "react";

export function useViewportHeight() {
  useEffect(() => {
    const setAppHeight = () => {
      const vh = window.visualViewport?.height ?? window.innerHeight;
      document.documentElement.style.setProperty("--app-height", `${vh}px`);
    };

    // On iOS Safari, focusing an off-screen input (like xterm's hidden textarea)
    // causes Safari to scroll the visual viewport, displacing the entire app.
    // Counter this by resetting scroll whenever the viewport is displaced.
    let resettingScroll = false;
    const resetViewportScroll = () => {
      if (resettingScroll) return;
      if (window.visualViewport && window.visualViewport.offsetTop > 0) {
        resettingScroll = true;
        window.scrollTo(0, 0);
        requestAnimationFrame(() => {
          resettingScroll = false;
        });
      }
    };

    setAppHeight();
    window.addEventListener("resize", setAppHeight);
    if (window.visualViewport) {
      window.visualViewport.addEventListener("resize", setAppHeight);
      window.visualViewport.addEventListener("scroll", setAppHeight);
      window.visualViewport.addEventListener("scroll", resetViewportScroll);
    }
    if ("orientation" in screen) {
      screen.orientation.addEventListener("change", setAppHeight);
    }
    return () => {
      window.removeEventListener("resize", setAppHeight);
      if (window.visualViewport) {
        window.visualViewport.removeEventListener("resize", setAppHeight);
        window.visualViewport.removeEventListener("scroll", setAppHeight);
        window.visualViewport.removeEventListener("scroll", resetViewportScroll);
      }
      if ("orientation" in screen) {
        screen.orientation.removeEventListener("change", setAppHeight);
      }
    };
  }, []);
}
