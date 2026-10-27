import { defineConfig } from "vite";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  server: {
    host: "localhost",
    port: 5173,
    strictPort: true,
  },
});
