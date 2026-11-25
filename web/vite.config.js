import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath } from "node:url";
async function appearanceBundle() {
  const { build } = await import("esbuild");
  const result = await build({
    entryPoints: [
      fileURLToPath(new URL("./src/appearance-init.js", import.meta.url)),
    ],
    bundle: true,
    write: false,
    format: "iife",
    minify: true,
  });
  return result.outputFiles[0].text;
}
export default defineConfig({
  plugins: [
    vue(),
    {
      name: "appearance-first-paint",
      transformIndexHtml: {
        order: "post",
        handler() {
          return [
            {
              tag: "script",
              attrs: { src: "/assets/appearance-init.js" },
              injectTo: "head-prepend",
            },
          ];
        },
      },
      configureServer(server) {
        server.middlewares.use(
          "/assets/appearance-init.js",
          async (_request, response, next) => {
            try {
              response.setHeader("Content-Type", "application/javascript");
              response.setHeader("Cache-Control", "no-store");
              response.end(await appearanceBundle());
            } catch (error) {
              next(error);
            }
          },
        );
      },
      async generateBundle() {
        this.emitFile({
          type: "asset",
          fileName: "assets/appearance-init.js",
          source: await appearanceBundle(),
        });
      },
    },
  ],
  server: {
    proxy: {
      "/api": { target: "http://127.0.0.1:8080", ws: true },
      "/healthz": "http://127.0.0.1:8080",
      "/install-client.sh": "http://127.0.0.1:8080",
      "/install-client.ps1": "http://127.0.0.1:8080",
      "/uninstall-client.sh": "http://127.0.0.1:8080",
      "/uninstall-client.ps1": "http://127.0.0.1:8080",
      "/install-agent.sh": "http://127.0.0.1:8080",
      "/install-agent.ps1": "http://127.0.0.1:8080",
    },
  },
});
