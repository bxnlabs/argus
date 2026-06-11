import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig(() => {
  const apiPort = process.env.ARGUS_PORT ?? "3000";
  const webPort = Number(process.env.ARGUS_WEB_PORT ?? "5273");
  const apiTarget = `http://localhost:${apiPort}`;
  const wsTarget = `ws://localhost:${apiPort}`;

  return {
    plugins: [react(), tailwindcss()],
    test: {
      environment: "jsdom",
      environmentOptions: {
        jsdom: {
          url: "http://localhost",
        },
      },
      setupFiles: ["./src/test-setup.ts"],
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    build: {
      outDir: path.resolve(__dirname, "../internal/web/dist"),
      emptyOutDir: true,
    },
    server: {
      port: webPort,
      proxy: {
        "/api/node/ws": {
          target: wsTarget,
          ws: true,
        },
        // Explicit registry entry. Without it, "/api/nodes" only reaches the
        // backend by prefix-matching the "/api/node" rule below — fragile if
        // that rule ever becomes an exact/regex match. Listed first so it wins.
        "/api/nodes": {
          target: apiTarget,
          changeOrigin: true,
        },
        "/api/node": {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
  };
});
