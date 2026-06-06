import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { githubKeys } from "./keys";

interface GitHubReposResponse {
  repos: string[];
}

export function useGitHubReposQuery(
  query: string,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: githubKeys.repos(query),
    queryFn: () => {
      const params = new URLSearchParams();
      if (query) params.set("q", query);
      const qs = params.toString();
      return apiFetch<GitHubReposResponse>(
        `/api/node/github/repos${qs ? `?${qs}` : ""}`,
      );
    },
    enabled: options?.enabled ?? true,
    staleTime: 60_000,
  });
}
