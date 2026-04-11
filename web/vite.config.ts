import path from "path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "../internal/site/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/admin/api": "http://localhost:7788",
      "/backend": "http://localhost:7788",
    },
  },
});
