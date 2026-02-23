import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch, getAgentBaseUrl } from "@/api/client";
import type { UploadResponse } from "@/types";
import { filesKeys } from "./keys";

export function useWriteFileMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ path, content }: { path: string; content: string }) => {
      return apiFetch<{ path: string; size: number }>(
        `/agent/api/files/content?path=${encodeURIComponent(path)}`,
        {
          method: "PUT",
          headers: { "Content-Type": "text/plain" },
          body: content,
        },
      );
    },
    onSuccess: (_data, variables) => {
      const fileName = variables.path.split("/").pop() || variables.path;
      toast.success(`Saved ${fileName}`);
      const lastSlash = variables.path.lastIndexOf("/");
      const parentDir = lastSlash > 0 ? variables.path.substring(0, lastSlash) : "";
      if (parentDir) {
        queryClient.invalidateQueries({ queryKey: filesKeys.list(parentDir) });
      }
    },
    onError: (error: Error) => {
      toast.error(error.message);
    },
  });
}

export function useFileUpload() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (files: File[]): Promise<UploadResponse> => {
      const formData = new FormData();
      for (const file of files) {
        formData.append("files", file);
      }

      const base = getAgentBaseUrl();
      const res = await fetch(`${base}/agent/api/files/upload`, {
        method: "POST",
        body: formData,
      });

      if (!res.ok) {
        let message = `Upload failed: ${res.status}`;
        try {
          const body = await res.json();
          if (body.error) message = body.error;
        } catch {
          // ignore parse errors
        }
        throw new Error(message);
      }

      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        predicate: (query) =>
          query.queryKey[0] === "files" &&
          query.queryKey[1] === "list" &&
          typeof query.queryKey[2] === "string" &&
          query.queryKey[2].startsWith("/tmp/argus-uploads"),
      });
    },
  });
}
