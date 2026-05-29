// copyToClipboard copies text using the async Clipboard API, falling back to a
// hidden-textarea execCommand for contexts where it is unavailable (e.g. argus
// served over plain HTTP, where navigator.clipboard is gated off as a non-secure
// context). Returns whether the copy is believed to have succeeded, so callers
// can show success/failure feedback.
//
// fallbackContainer must be supplied when the caller lives inside a focus trap
// (e.g. a Radix dialog). The fallback selects a hidden textarea, which moves
// focus to it; if that textarea is appended to document.body it sits outside the
// trap, so the trap synchronously yanks focus back and the copy silently copies
// nothing. Appending inside the trapped subtree keeps focus in-scope.
export async function copyToClipboard(
  text: string,
  fallbackContainer?: HTMLElement,
): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or insecure context — fall through to execCommand.
    }
  }
  return execCommandCopy(text, fallbackContainer);
}

function execCommandCopy(text: string, container?: HTMLElement): boolean {
  const host = container ?? document.body;
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  host.appendChild(textarea);
  textarea.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  textarea.remove();
  return ok;
}
