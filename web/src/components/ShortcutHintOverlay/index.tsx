import React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import type { ChordMap, ChordPending } from "@/hooks/useKeyboardChords";

interface ShortcutHintOverlayProps {
  pending: ChordPending | null;
  bindings: ChordMap;
  leaderLabel: string;
  helpOpen: boolean;
  onHelpOpenChange: (open: boolean) => void;
  extraShortcuts?: { keys: string; label: string }[];
}

/** Map special key names to display symbols; everything else is verbatim. */
function formatKey(key: string): string {
  if (key === "ArrowLeft") return "←";
  if (key === "ArrowRight") return "→";
  return key;
}

/** Pill-style key chip matching QuickSwitcher kbd conventions. */
function KeyChip({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        "bg-muted text-muted-foreground inline-flex items-center rounded border px-1.5 py-0.5 font-mono text-xs leading-none",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

/**
 * Walk `bindings` along `path` and join the labels of each step with " › ".
 * Stops early if a mid-path binding has no children (shouldn't happen — path
 * is always produced by descending into children). Returns null only when a
 * key is missing entirely.
 */
function buildBreadcrumb(bindings: ChordMap, path: string[]): string | null {
  let level: ChordMap = bindings;
  const labels: string[] = [];
  for (const key of path) {
    const binding = level[key];
    if (!binding) return null;
    labels.push(binding.label);
    if (!binding.children) break;
    level = binding.children;
  }
  return labels.join(" › ");
}

/** Flat list of rows for a single ChordMap level — used in both modes. */
function ChordRows({ level }: { level: ChordMap }) {
  return (
    <>
      {Object.entries(level).map(([key, binding]) => (
        <div key={key} className="flex items-center gap-2">
          <KeyChip>{formatKey(key)}</KeyChip>
          <span className="text-foreground/80 min-w-0 truncate text-xs">
            {binding.label}
          </span>
          {binding.children && (
            <span className="text-muted-foreground ml-auto text-xs">›</span>
          )}
        </div>
      ))}
    </>
  );
}

/** Recursively render the full chord tree for the reference modal. */
function ChordTreeRows({
  level,
  depth = 0,
}: {
  level: ChordMap;
  depth?: number;
}) {
  return (
    <>
      {Object.entries(level).map(([key, binding]) => (
        <div key={key}>
          <div
            className={cn(
              "flex items-center gap-2 py-0.5",
              depth > 0 && "pl-4",
            )}
          >
            <KeyChip>{formatKey(key)}</KeyChip>
            <span className="text-foreground/80 min-w-0 truncate text-xs">
              {binding.label}
            </span>
          </div>
          {binding.children && (
            <ChordTreeRows level={binding.children} depth={depth + 1} />
          )}
        </div>
      ))}
    </>
  );
}

export function ShortcutHintOverlay({
  pending,
  bindings,
  leaderLabel,
  helpOpen,
  onHelpOpenChange,
  extraShortcuts,
}: ShortcutHintOverlayProps): React.JSX.Element | null {
  // Nothing to show when idle and reference is closed.
  if (!pending && !helpOpen) return null;

  return (
    <>
      {/* ── Transient hint card (pointer-events-none, never traps focus) ── */}
      {pending && !helpOpen && (
        <div
          aria-live="polite"
          aria-label="Keyboard chord hints"
          className={cn(
            "fixed bottom-4 right-4 z-50 pointer-events-none",
            "bg-popover border-border text-popover-foreground",
            "rounded-lg border shadow-lg",
            "w-52 p-3",
          )}
        >
          {/* Heading: breadcrumb when inside a sub-chord, leader label at top level */}
          <p className="text-muted-foreground mb-2 text-xs font-medium">
            {pending.path.length === 0
              ? leaderLabel
              : (buildBreadcrumb(bindings, pending.path) ?? leaderLabel) + " ›"}
          </p>
          <div className="flex flex-col gap-1.5">
            <ChordRows level={pending.level} />
          </div>
        </div>
      )}

      {/* ── Pinned full reference (Dialog handles Esc / click-outside) ── */}
      <Dialog open={helpOpen} onOpenChange={onHelpOpenChange}>
        <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Keyboard Shortcuts</DialogTitle>
            <DialogDescription>
              Available keyboard shortcuts and chord sequences.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-3">
            {/* Leader */}
            <section>
              <p className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
                Leader
              </p>
              <div className="flex items-center gap-2">
                <KeyChip>{leaderLabel}</KeyChip>
                <span className="text-foreground/80 text-xs">Open command palette</span>
              </div>
            </section>

            {/* Chord tree */}
            <section>
              <p className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
                Chords
              </p>
              <div className="flex flex-col gap-0.5">
                <ChordTreeRows level={bindings} />
              </div>
            </section>

            {/* Extra (non-chord) shortcuts */}
            {extraShortcuts && extraShortcuts.length > 0 && (
              <section>
                <p className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
                  Other
                </p>
                <div className="flex flex-col gap-1">
                  {extraShortcuts.map(({ keys, label }) => (
                    <div key={keys} className="flex items-center gap-2">
                      <KeyChip>{keys}</KeyChip>
                      <span className="text-foreground/80 text-xs">{label}</span>
                    </div>
                  ))}
                </div>
              </section>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
