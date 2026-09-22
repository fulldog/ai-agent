import { fileURLToPath, URL } from "node:url";
import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";

const defaultProxyTarget = "http://127.0.0.1:18090";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, fileURLToPath(new URL(".", import.meta.url)), "");
  const target = (env.VITE_PROXY_TARGET || defaultProxyTarget).replace(/\/+$/, "");
  // dev 与 preview 各自独立解析 proxy，两处都要配，否则预览时 /api 会当静态资源处理。
  const proxy = {
    "/api": { target, changeOrigin: true },
    "/health": { target, changeOrigin: true },
  };

  return {
    plugins: [vue()],
    resolve: {
      alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
    },
    server: { host: true, port: 5173, proxy },
    preview: { port: 4173, proxy },
  };
});
