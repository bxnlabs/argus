import { useState, useEffect } from "react";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useAddNode, useUpdateNode } from "@/data/nodes/queries";
import { deriveNodeName } from "./deriveName";
import { isMac } from "@/lib/device";
import { useViewport } from "@/hooks/useViewport";
import type { NodeWithStatus } from "@/types";

type Scheme = "http" | "https";

// Split a stored node origin ("http://host[:port]") back into the scheme dropdown
// and host field for editing. Falls back to a blank http form on a parse miss.
function splitOrigin(rawUrl: string): { scheme: Scheme; host: string } {
  try {
    const u = new URL(rawUrl);
    if (u.protocol === "http:" || u.protocol === "https:") {
      return { scheme: u.protocol.slice(0, -1) as Scheme, host: u.host };
    }
  } catch {
    // fall through
  }
  return { scheme: "http", host: "" };
}

/**
 * Add or edit a manual (Custom) node. With no `node` it's an add form; with a
 * `node` it's prefilled for editing (the desktop rail / mobile panel open it via
 * each Custom node's action menu). Argus serves plain http over the tailnet
 * (Tailscale is the encryption layer), so http:// is the default scheme;
 * https:// is only for a custom node behind a TLS proxy — an explicit choice
 * rather than one guessed from the host.
 */
export function ManageNodesDialog({
  open,
  onClose,
  node,
}: {
  open: boolean;
  onClose: () => void;
  node?: NodeWithStatus | null;
}) {
  const addNode = useAddNode();
  const updateNode = useUpdateNode();
  const { isMobile } = useViewport();
  const [scheme, setScheme] = useState<Scheme>("http");
  const [host, setHost] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Seed the form when the dialog opens: from the edited node, or blank for add.
  useEffect(() => {
    if (!open) return;
    if (node) {
      const { scheme: s, host: h } = splitOrigin(node.url);
      setScheme(s);
      setHost(h);
      setName(node.name);
    } else {
      setScheme("http");
      setHost("");
      setName("");
    }
    setError(null);
  }, [open, node]);

  // Tolerate a pasted scheme/trailing slash in the host so the dropdown stays
  // the source of truth for the scheme.
  const cleanHost = host.trim().replace(/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//, "").replace(/\/+$/, "");
  const derivedName = deriveNodeName(cleanHost);
  const finalName = name.trim() || derivedName;
  const pending = addNode.isPending || updateNode.isPending;
  const canSubmit = !!cleanHost && !!finalName && !pending;

  const submit = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!canSubmit) return;
    setError(null);
    const url = `${scheme}://${cleanHost}`;
    try {
      if (node) {
        await updateNode.mutateAsync({ id: node.id, name: finalName, url });
      } else {
        await addNode.mutateAsync({ name: finalName, url });
      }
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save node");
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="top-[env(safe-area-inset-top)] translate-y-0 max-h-[85vh] overflow-y-auto sm:top-[50%] sm:translate-y-[-50%]"
        // Cmd/Ctrl+Enter saves from anywhere in the dialog. Capture phase keeps
        // it ahead of the scheme Select, which opens on a bare Enter.
        onKeyDownCapture={(e) => {
          if (!e.currentTarget.contains(e.target as Node)) return;
          if (e.nativeEvent.isComposing) return;
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            e.stopPropagation();
            submit();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>Configure Node</DialogTitle>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Host</label>
            <div className="flex gap-2">
              <Select value={scheme} onValueChange={(v) => setScheme(v as Scheme)}>
                <SelectTrigger className="w-[104px] flex-shrink-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="http">http://</SelectItem>
                  <SelectItem value="https">https://</SelectItem>
                </SelectContent>
              </Select>
              <Input
                value={host}
                onChange={(e) => setHost(e.target.value)}
                autoFocus
              />
            </div>
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Name
              <span className="text-muted-foreground font-normal"> (optional)</span>
            </label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={derivedName}
            />
          </div>

          {error && <p className="text-destructive text-sm">{error}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              Save
              {!isMobile && (
                <kbd
                  aria-hidden="true"
                  className="bg-primary-foreground/15 hidden rounded px-1 py-0.5 text-[10px] sm:inline-block"
                >
                  {isMac() ? "⌘ ↵" : "Ctrl ↵"}
                </kbd>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
