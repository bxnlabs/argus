export function getStatusColor(status?: string) {
  switch (status) {
    case "active":
      return "bg-green-500";
    case "idle":
      return "bg-muted-foreground";
    case "dead":
      return "bg-red-500/50";
    default:
      return "bg-muted-foreground/40";
  }
}

export function getStatusAnimation(status?: string) {
  switch (status) {
    case "active":
      return "animate-pulse-green";
    default:
      return "";
  }
}

export function getStatusLabel(status?: string) {
  switch (status) {
    case "active":
      return "Active";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    default:
      return "";
  }
}
