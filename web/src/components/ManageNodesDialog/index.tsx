import { useState } from "react";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Trash2 } from "lucide-react";
import { useNodeContext } from "@/contexts/NodeContext";
import { useAddNode, useDeleteNode } from "@/data/nodes/queries";

export function ManageNodesDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { nodes } = useNodeContext();
  const addNode = useAddNode();
  const deleteNode = useDeleteNode();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);

  const manual = nodes.filter((n) => n.source === "manual");
  const discovered = nodes.filter((n) => n.source === "discovered");

  const submit = async () => {
    setError(null);
    try {
      await addNode.mutateAsync({ name: name.trim(), url: url.trim() });
      setName("");
      setUrl("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to add node");
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Manage nodes</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          <Input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <Input
            placeholder="http://host:80"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && name.trim() && url.trim()) submit(); }}
          />
          {error && <p className="text-destructive text-sm">{error}</p>}
          <Button onClick={submit} disabled={!name.trim() || !url.trim() || addNode.isPending}>
            Add node
          </Button>
        </div>

        {manual.length > 0 && (
          <div className="mt-2 flex flex-col gap-1">
            <p className="text-muted-foreground text-xs font-bold">ADDED</p>
            {manual.map((n) => (
              <div key={n.id} className="flex items-center gap-2 text-sm">
                <span className="flex-1 truncate">{n.name}</span>
                <span className="text-muted-foreground truncate text-xs">{n.url}</span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Remove ${n.name}`}
                  onClick={() => deleteNode.mutate(n.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {discovered.length > 0 && (
          <div className="mt-2 flex flex-col gap-1">
            <p className="text-muted-foreground text-xs font-bold">DISCOVERED</p>
            {discovered.map((n) => (
              <div key={n.id} className="text-muted-foreground flex items-center gap-2 text-sm">
                <span className="flex-1 truncate">{n.name}</span>
                <span className="truncate text-xs">{n.url}</span>
              </div>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
