import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { UploadResponse } from "@/types";

export function useFileUpload() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: async (files: File[]): Promise<UploadResponse> => {
      const formData = new FormData();
      for (const file of files) {
        formData.append("files", file);
      }

      // Raw multipart POST (not apiFetch, which sets a JSON Content-Type).
      const res = await fetch(`${baseUrl}/api/node/files/upload`, {
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
      // Invalidate this node's upload-dir listings. Keys are
      // ["files", scope, "list", path]; match scope + the uploads path prefix.
      queryClient.invalidateQueries({
        predicate: (query) =>
          query.queryKey[0] === "files" &&
          query.queryKey[1] === scope &&
          query.queryKey[2] === "list" &&
          typeof query.queryKey[3] === "string" &&
          query.queryKey[3].startsWith("/tmp/argus-uploads"),
      });
    },
  });
}
