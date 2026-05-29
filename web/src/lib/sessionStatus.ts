export interface StatusMeta {
  label: string;
  color: string;
  animation: string;
}

const STATUS: Record<string, StatusMeta> = {
  active: { label: "Active", color: "bg-green-500", animation: "animate-pulse-green" },
  idle: { label: "Idle", color: "bg-muted-foreground", animation: "" },
  dead: { label: "Dead", color: "bg-red-500/50", animation: "" },
};

const UNKNOWN: StatusMeta = {
  label: "",
  color: "bg-muted-foreground/40",
  animation: "",
};

// getStatusMeta maps a session status to its dot color, pulse animation, and
// human label. Unknown or undefined statuses get a muted, unlabeled fallback.
export function getStatusMeta(status?: string): StatusMeta {
  return (status && STATUS[status]) || UNKNOWN;
}
