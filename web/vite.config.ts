import { defineConfig } from "vite";
import preact from "@preact/preset-vite";

const api = {
  target: "http://127.0.0.1:7878",
  changeOrigin: true,
  // The Go server rejects cross-origin writes; in dev the page origin is Vite's, so drop it at the proxy.
  configure: (proxy: any) => proxy.on("proxyReq", (req: any) => req.removeHeader("origin")),
};

export default defineConfig({
  plugins: [preact()],
  server: { port: 5173, proxy: { "/api": api, "/assets": api, "/library": api } },
  build: { outDir: "dist", assetsDir: "app", emptyOutDir: true, chunkSizeWarningLimit: 1600 },
});
