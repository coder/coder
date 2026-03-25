import * as path from "node:path";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import react from "@vitejs/plugin-react";
import { playwright } from "@vitest/browser-playwright";
import { visualizer } from "rollup-plugin-visualizer";
import type { PluginOption } from "vite";
import checker from "vite-plugin-checker";
import { defineConfig } from "vitest/config";

// Enable the React profiling build and discoverable source maps for
// internal deployments (e.g. dogfood). The profiling build swaps
// react-dom/client for react-dom/profiling, which keeps production
// optimizations but leaves the <Profiler> onRender callback and
// React Performance Tracks instrumentation intact. The overhead is
// ~13% on the react-dom chunk size.
const isProfilingBuild = process.env.CODER_REACT_PROFILING === "true";

const plugins: PluginOption[] = [
	react({
		babel: {
			plugins: [],
			overrides: [
				{
					test: /src\/(pages\/AgentsPage|components\/ai-elements)\//,
					plugins: ["babel-plugin-react-compiler"],
				},
			],
		},
	}),
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
	worker: {
		format: "es",
	},
	publicDir: path.resolve(__dirname, "./static"),
	build: {
		outDir: path.resolve(__dirname, "./out"),
		emptyOutDir: false, // We need to keep the /bin folder and GITKEEP files
		sourcemap: isProfilingBuild ? true : "hidden",
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
					if (process.env.CODER_SESSION_TOKEN) {
						proxy.on("proxyReq", (proxyReq) => {
							proxyReq.setHeader(
								"Coder-Session-Token",
								process.env.CODER_SESSION_TOKEN!,
							);
						});
					}
					// Vite does not catch socket errors, and stops the webserver.
					// As /logs endpoint can return HTTP 4xx status, we need to embrace
					// Vite with a custom error handler to prevent from quitting.
					proxy.on("proxyReqWs", (proxyReq, _req, socket) => {
						if (process.env.NODE_ENV === "development") {
							proxyReq.setHeader(
								"origin",
								process.env.CODER_HOST || "http://localhost:3000",
							);
							if (process.env.CODER_SESSION_TOKEN) {
								proxyReq.setHeader(
									"Coder-Session-Token",
									process.env.CODER_SESSION_TOKEN!,
								);
							}
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
	// Pre-bundle deps that Vite tends to discover late (deep MUI
	// imports, Emotion). Without this, Vite re-optimizes mid-session
	// which returns 504 "Outdated Optimize Dep" for every previously
	// served chunk, cascading into dynamic import failures.
	optimizeDeps: {
		include: [
			"@emotion/cache",
			"@emotion/css",
			"@emotion/react",
			"@emotion/react/jsx-runtime",
			"@emotion/styled",
			"@mui/material/Autocomplete",
			"@mui/material/Card",
			"@mui/material/CardActionArea",
			"@mui/material/CardContent",
			"@mui/material/Checkbox",
			"@mui/material/CircularProgress",
			"@mui/material/Collapse",
			"@mui/material/CssBaseline",
			"@mui/material/Dialog",
			"@mui/material/DialogActions",
			"@mui/material/DialogContent",
			"@mui/material/DialogContentText",
			"@mui/material/DialogTitle",
			"@mui/material/Divider",
			"@mui/material/Drawer",
			"@mui/material/FormControl",
			"@mui/material/FormControlLabel",
			"@mui/material/FormGroup",
			"@mui/material/FormHelperText",
			"@mui/material/FormLabel",
			"@mui/material/IconButton",
			"@mui/material/InputAdornment",
			"@mui/material/InputBase",
			"@mui/material/LinearProgress",
			"@mui/material/Link",
			"@mui/material/List",
			"@mui/material/ListItem",
			"@mui/material/ListItemText",
			"@mui/material/Menu",
			"@mui/material/MenuItem",
			"@mui/material/MenuList",
			"@mui/material/Radio",
			"@mui/material/RadioGroup",
			"@mui/material/Select",
			"@mui/material/Skeleton",
			"@mui/material/Snackbar",
			"@mui/material/Stack",
			"@mui/material/SvgIcon",
			"@mui/material/TableRow",
			"@mui/material/TextField",
			"@mui/material/ToggleButton",
			"@mui/material/ToggleButtonGroup",
			"@mui/material/styles",
			"@mui/system/createTheme",
			"@mui/system/useTheme",
			"@mui/x-tree-view",
		],
	},
	resolve: {
		alias: {
			// In profiling builds, swap the production react-dom client
			// bundle for the profiling variant so that <Profiler>
			// onRender receives actual timing data.
			// Note: react-dom/profiling is a superset of react-dom/client
			// (16 vs 3 exports). If a future React major changes this
			// relationship, the alias may need updating.
			...(isProfilingBuild
				? { "react-dom/client": "react-dom/profiling" }
				: {}),
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
		silent: "passed-only",
		projects: [
			{
				extends: true,
				test: {
					name: "unit",
					include: ["src/**/*.test.?(m)ts?(x)"],
					globals: true,
					environment: "jsdom",
					setupFiles: [
						"@testing-library/jest-dom/vitest",
						"./test/vitestSetup.ts",
					],
				},
			},
			// Storybook story tests via Playwright browser mode.
			// Discovery handled by the storybookTest plugin via
			// .storybook/main.ts `stories` config.
			{
				extends: true,
				plugins: [
					storybookTest({
						configDir: path.join(__dirname, ".storybook"),
					}),
				],
				test: {
					name: "storybook",
					browser: {
						enabled: true,
						headless: true,
						provider: playwright(),
						instances: [{ browser: "chromium" }],
					},
					setupFiles: [".storybook/vitest.setup.ts"],
				},
			},
		],
	},
});
