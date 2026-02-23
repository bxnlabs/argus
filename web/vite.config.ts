import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
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
    proxy: {
      "/agent/api": {
        target: "http://localhost:3000",
        changeOrigin: true,
      },
      "/agent/ws": {
        target: "ws://localhost:3000",
        ws: true,
      },
      "/api": {
        target: "http://localhost:3000",
        changeOrigin: true,
      },
    },
  },
});
