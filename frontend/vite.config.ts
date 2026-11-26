import path from "path";
import { fileURLToPath } from "url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { viteSingleFile } from "vite-plugin-singlefile";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), viteSingleFile()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "react-i18next": path.resolve(__dirname, "src/i18n.ts"),
      i18next: path.resolve(__dirname, "src/i18n.ts"),
    },
  },
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
    target: "es2022",
    cssMinify: true,
  },
  esbuild: {
    legalComments: "none",
  },
  server: {
    proxy: {
      "/api": "http://localhost:19100",
    },
  },
});
