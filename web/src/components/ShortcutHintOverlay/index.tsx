import React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import type {
  ChordBinding,
  ChordMap,
  ChordPending,
} from "@/hooks/useKeyboardChords";

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
 * A row to render: either a single binding or a collapsed group of adjacent
 * bindings sharing a `collapse` token (e.g. the 1–9 node-switch chords), shown
 * as one row with a key range.
 */
type DisplayRow =
  | { kind: "single"; key: string; binding: ChordBinding }
  | { kind: "group"; keys: string[]; label: string; collapse: string };

/**
 * Fold a level's entries into display rows, merging runs of bindings that share
 * a `collapse` token into one group row. Order is preserved; only consecutive
 * same-token bindings merge (node chords are emitted contiguously).
 */
function toDisplayRows(level: ChordMap): DisplayRow[] {
  const rows: DisplayRow[] = [];
  for (const [key, binding] of Object.entries(level)) {
    const prev = rows[rows.length - 1];
    if (
      binding.collapse &&
      prev?.kind === "group" &&
      prev.collapse === binding.collapse
    ) {
      prev.keys.push(key);
    } else if (binding.collapse) {
      rows.push({
        kind: "group",
        keys: [key],
        label: binding.label,
        collapse: binding.collapse,
      });
    } else {
      rows.push({ kind: "single", key, binding });
    }
  }
  return rows;
}

/**
 * Render a group's keys as a single range chip ("1–9") when they form a
 * contiguous numeric run, otherwise list them ("1, 3, 5") so the chip never
 * implies keys that aren't bound. A lone key renders verbatim.
 */
function GroupKeyChip({ keys }: { keys: string[] }): React.JSX.Element {
  const nums = keys.map(Number);
  const contiguous =
    nums.every(Number.isInteger) &&
    nums.every((n, i) => i === 0 || n === nums[i - 1] + 1);
  const text =
    keys.length > 1 && contiguous
      ? `${keys[0]}–${keys[keys.length - 1]}`
      : keys.join(", ");
  return <KeyChip>{text}</KeyChip>;
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
      {toDisplayRows(level).map((row) =>
        row.kind === "group" ? (
          <div key={row.keys[0]} className="flex items-center gap-2">
            <GroupKeyChip keys={row.keys} />
            <span className="text-foreground/80 min-w-0 truncate text-xs">
              {row.label}
            </span>
          </div>
        ) : (
          <div key={row.key} className="flex items-center gap-2">
            <KeyChip>{formatKey(row.key)}</KeyChip>
            <span className="text-foreground/80 min-w-0 truncate text-xs">
              {row.binding.label}
            </span>
            {row.binding.children && (
              <span className="text-muted-foreground ml-auto text-xs">›</span>
            )}
          </div>
        ),
      )}
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
      {toDisplayRows(level).map((row) => (
        <div key={row.kind === "group" ? row.keys[0] : row.key}>
          <div
            className={cn(
              "flex items-center gap-2 py-0.5",
              depth > 0 && "pl-4",
            )}
          >
            {row.kind === "group" ? (
              <GroupKeyChip keys={row.keys} />
            ) : (
              <KeyChip>{formatKey(row.key)}</KeyChip>
            )}
            <span className="text-foreground/80 min-w-0 truncate text-xs">
              {row.kind === "group" ? row.label : row.binding.label}
            </span>
          </div>
          {row.kind === "single" && row.binding.children && (
            <ChordTreeRows level={row.binding.children} depth={depth + 1} />
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
          {/* Heading: leader chip + title at top level, breadcrumb in a sub-chord */}
          {pending.path.length === 0 ? (
            <div className="mb-2 flex items-center justify-between gap-2">
              <span className="text-foreground text-xs font-semibold">
                Keyboard Shortcuts
              </span>
              <KeyChip>{leaderLabel}</KeyChip>
            </div>
          ) : (
            <p className="text-muted-foreground mb-2 text-xs font-medium">
              {(buildBreadcrumb(bindings, pending.path) ?? leaderLabel) + " ›"}
            </p>
          )}
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
                <span className="text-foreground/80 text-xs">Show shortcuts</span>
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
