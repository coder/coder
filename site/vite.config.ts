import react from "@vitejs/plugin-react";
import path from "path";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig, type PluginOption } from "vite";
import checker from "vite-plugin-checker";

const plugins: PluginOption[] = [
  react(),
  checker({
    typescript: true,
  }),
];

if (process.env.STATS !== undefined) {
  plugins.push(
    visualizer({
      filename: "./stats/index.html",
    }),
  );
}

export default defineConfig({
  plugins: plugins,
  publicDir: path.resolve(__dirname, "./static"),
  build: {
    outDir: path.resolve(__dirname, "./out"),
    // We need to keep the /bin folder and GITKEEP files
    emptyOutDir: false,
    // 'hidden' works like true except that the corresponding sourcemap comments in the bundled files are suppressed
    sourcemap: "hidden",
  },
  define: {
    "process.env": {
      NODE_ENV: process.env.NODE_ENV,
      STORYBOOK: process.env.STORYBOOK,
      INSPECT_XSTATE: process.env.INSPECT_XSTATE,
    },
  },
  server: {
    port: process.env.PORT ? Number(process.env.PORT) : 8080,
    headers: {
      // This header corresponds to "src/api/api.ts"'s hardcoded FE token.
      // This is the secret side of the CSRF double cookie submit method.
      // This should be sent on **every** response from the webserver.
      //
      // This is required because in production, the Golang webserver generates
      // this "Set-Cookie" header. The Vite webserver needs to replicate this
      // behavior. Instead of implementing CSRF though, we just use static
      // values for simplicity.
      "Set-Cookie":
        "csrf_token=JXm9hOUdZctWt0ZZGAy9xiS/gxMKYOThdxjjMnMUyn4=; Path=/; HttpOnly; SameSite=Lax",
    },
    proxy: {
      "/api": {
        ws: true,
        changeOrigin: true,
        target: process.env.CODER_HOST || "http://localhost:3000",
        secure: process.env.NODE_ENV === "production",
        configure: (proxy) => {
          // Vite does not catch socket errors, and stops the webserver.
          // As /logs endpoint can return HTTP 4xx status, we need to embrace
          // Vite with a custom error handler to prevent from quitting.
          proxy.on("proxyReqWs", (proxyReq, req, socket) => {
            if (process.env.NODE_ENV === "development") {
              proxyReq.setHeader(
                "origin",
                process.env.CODER_HOST || "http://localhost:3000",
              );
            }

            socket.on("error", (error) => {
              console.error(error);
            });
          });
        },
      },
      "/swagger": {
        target: process.env.CODER_HOST || "http://localhost:3000",
        secure: process.env.NODE_ENV === "production",
      },
    },
  },
  resolve: {
    alias: {
      api: path.resolve(__dirname, "./src/api"),
      components: path.resolve(__dirname, "./src/components"),
      contexts: path.resolve(__dirname, "./src/contexts"),
      hooks: path.resolve(__dirname, "./src/hooks"),
      modules: path.resolve(__dirname, "./src/modules"),
      pages: path.resolve(__dirname, "./src/pages"),
      testHelpers: path.resolve(__dirname, "./src/testHelpers"),
      theme: path.resolve(__dirname, "./src/theme"),
      utils: path.resolve(__dirname, "./src/utils"),
    },
  },
});
