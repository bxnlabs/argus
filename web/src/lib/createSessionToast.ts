export interface CreateToastAction {
  label: string;
  onClick: () => void;
}

export interface CreateToastOptions {
  id?: string | number;
  action?: CreateToastAction;
  duration?: number;
}

export interface CreateToastDecision {
  kind: "success" | "error";
  message: string;
  options: CreateToastOptions;
}

// The decision table behind App's create-session toast: what (if anything)
// to show, given the outcome, the id of a handoff toast raised if the dialog
// was dismissed mid-create (null if it wasn't), and an optional "Open" action
// for a session that could not be attached to the tab that started it. Pure
// and side-effect-free so the branching can be tested without sonner or App.
export function resolveCreateToastDecision(
  outcome: "success" | "error",
  name: string,
  id: string | number | null,
  action?: CreateToastAction,
): CreateToastDecision | null {
  if (outcome === "error") {
    return {
      kind: "error",
      message: "Failed to create session",
      options: id !== null ? { id } : {},
    };
  }

  // Nothing to say: the dialog was never dismissed and the session landed
  // where it was meant to.
  if (id === null && !action) return null;

  return {
    kind: "success",
    message: `Created ${name}`,
    options: {
      ...(id !== null ? { id } : {}),
      // Actionable toasts get a longer duration than sonner's ~4s default:
      // the "Open" action is the only recovery affordance and the user may
      // be looking at another tab when it appears.
      ...(action ? { action, duration: 10000 } : {}),
    },
  };
}
