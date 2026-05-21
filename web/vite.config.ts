import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Vite dev server runs on 5173 and proxies /api to the Go backend on 8080.
// Production builds emit to ../internal/server/dist so the Go binary embeds them.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        // SSE + MJPEG are long-lived; keep them streaming.
        ws: false,
      },
    },
  },
  build: {
    outDir: path.resolve(__dirname, "../internal/server/dist"),
    emptyOutDir: true,
  },
});
