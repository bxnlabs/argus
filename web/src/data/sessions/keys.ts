export const sessionKeys = {
  all: ["sessions"] as const,
  list: () => [...sessionKeys.all, "list"] as const,
};

export const statusKeys = {
  all: ["session-statuses"] as const,
};

export const profileKeys = {
  all: ["profiles"] as const,
  list: () => [...profileKeys.all, "list"] as const,
};
