import react from "@vitejs/plugin-react"
import path from "path"
import { defineConfig, PluginOption } from "vite"
import { visualizer } from "rollup-plugin-visualizer"

const plugins: PluginOption[] = [react()]

if (process.env.STATS !== undefined) {
  plugins.push(
    visualizer({
      filename: "./stats/index.html",
    }),
  )
}

export default defineConfig({
  plugins: plugins,
  publicDir: path.resolve(__dirname, "./static"),
  build: {
    outDir: path.resolve(__dirname, "./out"),
    // We need to keep the /bin folder and GITKEEP files
    emptyOutDir: false,
    sourcemap: process.env.NODE_ENV === "development",
  },
  define: {
    "process.env": {
      NODE_ENV: process.env.NODE_ENV,
      INSPECT_XSTATE: process.env.INSPECT_XSTATE,
    },
  },
  server: {
    port: process.env.PORT ? Number(process.env.PORT) : 8080,
    proxy: {
      "/api": {
        target: process.env.CODER_HOST || "http://localhost:3000",
        ws: true,
        secure: process.env.NODE_ENV === "production",
      },
      "/swagger": {
        target: process.env.CODER_HOST || "http://localhost:3000",
        secure: process.env.NODE_ENV === "production",
      },
    },
  },
  resolve: {
    alias: [
      { find: "api", replacement: path.resolve(__dirname, "./src/api") },
      {
        find: "components",
        replacement: path.resolve(__dirname, "./src/components"),
      },
      { find: "hooks", replacement: path.resolve(__dirname, "./src/hooks") },
      { find: "i18n", replacement: path.resolve(__dirname, "./src/i18n") },
      { find: "pages", replacement: path.resolve(__dirname, "./src/pages") },
      {
        find: "testHelpers",
        replacement: path.resolve(__dirname, "./src/testHelpers"),
      },
      { find: "theme", replacement: path.resolve(__dirname, "./src/theme") },
      { find: /^util$/, replacement: path.resolve("./node_modules/util") },
      { find: "util", replacement: path.resolve(__dirname, "./src/util") },
      {
        find: "xServices",
        replacement: path.resolve(__dirname, "./src/xServices"),
      },
    ],
  }
})
