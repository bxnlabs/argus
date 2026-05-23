import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig(() => {
  const apiPort = process.env.ARGUS_SERVER_PORT ?? "3000";
  const webPort = Number(process.env.ARGUS_WEB_PORT ?? "5273");
  const apiTarget = `http://localhost:${apiPort}`;
  const wsTarget = `ws://localhost:${apiPort}`;

  return {
    plugins: [react(), tailwindcss()],
    test: {
      environment: "jsdom",
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
        "/node/api": {
          target: apiTarget,
          changeOrigin: true,
        },
        "/node/ws": {
          target: wsTarget,
          ws: true,
        },
        "/api": {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
  };
});
