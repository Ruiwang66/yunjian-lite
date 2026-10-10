import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
export default defineConfig({
  plugins: [vue()],
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
