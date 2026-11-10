// Сборка приложения. Отдаёт его Go-сервер с корня и только по сессии
// (handleRoot в server/main.go), а API в разработке проксируется на
// локальный сервер: go run ./server -root . -addr 127.0.0.1:8642
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const go = "http://127.0.0.1:8642";
// те же экраны, что isAppPath в server/main.go
const APP = /^\/(doc|folder|tag|edit)\/|^\/(search|access|new|inbox|health|connections)?(\?|$)/;

export default defineConfig({
  base: "/",
  plugins: [react()],
  server: {
    proxy: {
      // всё, кроме самого приложения, — на Go: API, формы, шрифты, вход.
      // Экраны приложения (GET страницы) отдаёт Vite, как Go отдаёт index.html.
      "^/(?!@|src/|node_modules/).*": {
        target: go,
        changeOrigin: false,
        bypass: (req) =>
          req.method === "GET" && req.headers.accept?.includes("text/html") && APP.test(req.url ?? "")
            ? "/index.html"
            : undefined,
      },
    },
  },
  build: { outDir: "dist", emptyOutDir: true, sourcemap: false },
});
