import * as path from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import type { PluginOption } from "vite";
import checker from "vite-plugin-checker";
import { defineConfig } from "vitest/config";

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
			gzipSize: true,
		}),
	);
}

export default defineConfig({
	plugins,
	publicDir: path.resolve(__dirname, "./static"),
	build: {
		outDir: path.resolve(__dirname, "./out"),
		emptyOutDir: false, // We need to keep the /bin folder and GITKEEP files
		sourcemap: "hidden",
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
				manualChunks(id) {
					if (!id.includes("node_modules")) {
						return;
					}

					if (id.includes("@mui")) return "mui";
					if (id.includes("@emotion")) return "emotion";
					if (id.includes("monaco-editor")) return "monaco";
					if (id.includes("@xterm")) return "xterm";
					if (id.includes("emoji-mart")) return "emoji-mart";
					if (id.includes("radix-ui")) return "radix-ui";
				},
			},
		},
	},
	define: {
		"process.env": {
			NODE_ENV: process.env.NODE_ENV,
			STORYBOOK: process.env.STORYBOOK,
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
					proxy.on("proxyReqWs", (proxyReq, _req, socket) => {
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
		allowedHosts: [".coder", ".dev.coder.com"],
	},
	resolve: {
		alias: {
			App: path.resolve(__dirname, "./src/App"),
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
	test: {
		include: ["src/**/*.test.?(m)ts?(x)"],
		globals: true,
		environment: "jsdom",
		setupFiles: ["@testing-library/jest-dom/vitest"],
		silent: "passed-only",
	},
});
