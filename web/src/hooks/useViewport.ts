import { useState, useEffect } from "react";
import { isTouchDevice, isPhoneSized } from "@/lib/device";

export function useViewport() {
  const [isMobile, setIsMobile] = useState(
    () => isTouchDevice && isPhoneSized()
  );
  const [isHydrated, setIsHydrated] = useState(false);

  useEffect(() => {
    // A phone is a touch-primary device with a short dimension under 768px.
    // Using the short side means landscape rotation can't escape mobile mode.
    const checkViewport = () =>
      setIsMobile(isTouchDevice && isPhoneSized());
    checkViewport();
    setIsHydrated(true);
    window.addEventListener("resize", checkViewport);
    return () => window.removeEventListener("resize", checkViewport);
  }, []);

  return { isMobile, isDesktop: !isMobile, isHydrated };
}
