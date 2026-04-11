import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/site/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/admin/api": "http://localhost:7788",
      "/oneapi": "http://localhost:7788",
    },
  },
});
