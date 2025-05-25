import * as path from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { type PluginOption, defineConfig } from "vite";
import checker from "vite-plugin-checker";

// Define plugins with conditional additions
const plugins: PluginOption[] = [
  // React plugin with improved options
  react({
    // Enable Fast Refresh for development
    fastRefresh: true,
    // Use Babel only for production builds
    babel: {
      // Only use Babel for production builds (faster dev builds)
      skipEnvCheck: process.env.NODE_ENV !== "production",
    },
  }),
  // TypeScript checking
  checker({
    typescript: true,
    // Enable ESLint checking only in development to catch issues early
    // Comment out if it slows down too much
    eslint: process.env.NODE_ENV === "development" ? {
      lintCommand: "biome lint --error-on-warnings .",
    } : false,
  }),
];

// Add bundle analyzer in stats mode
if (process.env.STATS !== undefined) {
  plugins.push(
    visualizer({
      filename: "./stats/index.html",
      gzipSize: true,
      brotliSize: true,
      open: false,
    }),
  );
}

export default defineConfig({
  plugins,
  publicDir: path.resolve(__dirname, "./static"),
  build: {
    outDir: path.resolve(__dirname, "./out"),
    // We need to keep the /bin folder and GITKEEP files
    emptyOutDir: false,
    // 'hidden' works like true except that the corresponding sourcemap comments in the bundled files are suppressed
    sourcemap: "hidden",
    // Optimize build performance and output
    minify: "terser",
    terserOptions: {
      compress: {
        drop_console: process.env.NODE_ENV === "production",
        drop_debugger: process.env.NODE_ENV === "production",
      },
    },
    // Chunk splitting strategy
    rollupOptions: {
      input: {
        index: path.resolve(__dirname, "./index.html"),
        serviceWorker: path.resolve(__dirname, "./src/serviceWorker.ts"),
      },
      output: {
        entryFileNames: (chunkInfo) => {
          return chunkInfo.name === "serviceWorker"
            ? "[name].js"
            : "assets/[name]-[hash].js";
        },
        // Optimize chunks
        manualChunks(id) {
          // Create separate chunks for large dependencies
          if (id.includes("node_modules")) {
            if (id.includes("@mui")) return "vendor-mui";
            if (id.includes("@emotion")) return "vendor-emotion";
            if (id.includes("react") || id.includes("react-dom")) return "vendor-react";
            if (id.includes("monaco-editor")) return "vendor-monaco";
            if (id.includes("@xterm")) return "vendor-xterm";
            // All other dependencies in a shared vendor bundle
            return "vendor";
          }
        },
      },
    },
    // Add environment variable
    reportCompressedSize: process.env.STATS !== undefined,
    // Target modern browsers only (as specified in package.json)
    target: "es2020",
  },
  define: {
    "process.env": {
      NODE_ENV: process.env.NODE_ENV,
      STORYBOOK: process.env.STORYBOOK,
      INSPECT_XSTATE: process.env.INSPECT_XSTATE,
    },
  },
  server: {
    host: "127.0.0.1",
    port: process.env.PORT ? Number(process.env.PORT) : 8080,
    // Optimize dev server
    hmr: {
      overlay: true,
    },
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
      "//": {
        changeOrigin: true,
        target: process.env.CODER_HOST || "http://localhost:3000",
        secure: process.env.NODE_ENV === "production",
        rewrite: (path) => path.replace(/\/+/g, "/"),
      },
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
      "/healthz": {
        target: process.env.CODER_HOST || "http://localhost:3000",
        secure: process.env.NODE_ENV === "production",
      },
      "/serviceWorker.js": {
        target: process.env.CODER_HOST || "http://localhost:3000",
        secure: process.env.NODE_ENV === "production",
      },
    },
    allowedHosts: true,
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
  // Add caching optimization
  optimizeDeps: {
    // Exclude large packages that don't need to be pre-bundled
    exclude: ['monaco-editor'],
    esbuildOptions: {
      target: 'es2020',
    },
  },
  // Add CSS optimization
  css: {
    devSourcemap: true,
    // Add module CSS support for Tailwind
    modules: {
      localsConvention: 'camelCaseOnly',
    },
  },
  // Enable asset inlining (small assets will be inlined as data URLs)
  assetsInclude: ['**/*.woff2', '**/*.woff'],
});