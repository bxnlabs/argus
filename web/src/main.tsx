import React from "react";
import ReactDOM from "react-dom/client";
import { ErrorBoundary, type FallbackProps } from "react-error-boundary";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/query-client";
import { App } from "@/App";
import "./globals.css";

function ErrorFallback({ error }: FallbackProps) {
  return (
    <div className="flex h-dvh flex-col items-center justify-center gap-4 bg-background p-8 text-foreground">
      <h1 className="text-lg font-semibold">Something went wrong</h1>
      <pre className="max-w-lg overflow-auto rounded bg-muted p-4 text-sm text-muted-foreground">
        {error instanceof Error ? error.message : String(error)}
      </pre>
      <button
        onClick={() => window.location.reload()}
        className="rounded bg-primary px-4 py-2 text-sm text-primary-foreground"
      >
        Reload
      </button>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary FallbackComponent={ErrorFallback}>
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </ErrorBoundary>
  </React.StrictMode>
);
