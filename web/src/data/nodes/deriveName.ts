// Derive a friendly node name from a host. This reverses how a node's default
// Tailscale hostname is built (`argus-<hostname>`, see cmd/argus/main.go): take
// the first DNS label and strip a leading `argus-`. So
// "argus-bumblebee.tail06de7.ts.net" → "bumblebee". Tolerates a pasted scheme,
// path, or port so it works on whatever the user types into the Host field.
export function deriveNodeName(host: string): string {
  let h = host.trim();
  if (!h) return "";
  // Drop a pasted scheme ("http://", "https://", …).
  h = h.replace(/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//, "");
  // Drop any path/query, then a trailing :port.
  h = h.split(/[/?#]/)[0].replace(/:\d+$/, "");
  // First DNS label, with the default argus- hostname prefix stripped.
  return h.split(".")[0].replace(/^argus-/, "");
}
