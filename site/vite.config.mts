import { EventEmitter } from "node:events";
import * as http from "node:http";
import * as https from "node:https";
import * as net from "node:net";
import * as path from "node:path";
import * as stream from "node:stream";
import * as tls from "node:tls";
import babel from "@rolldown/plugin-babel";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import tailwindcss from "@tailwindcss/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import { playwright } from "@vitest/browser-playwright";
import { visualizer } from "rollup-plugin-visualizer";
import { build, type PluginOption } from "vite";
import checker from "vite-plugin-checker";
import { defineConfig } from "vitest/config";

// We enable profiling and source maps for internal deployments (e.g. dogfood).
// The profiling build uses react-dom/profiling, which keeps optimizations but
// preserves performance instrumentation.
const isProfilingBuild = process.env.CODER_REACT_PROFILING === "true";

const compilerPreset = reactCompilerPreset();
compilerPreset.rolldown.filter = {
	...compilerPreset.rolldown.filter,
	id: {
		// Keep in sync with targetDirs in scripts/check-compiler.mjs.
		include: [
			/src\/modules\/aiModels\//,
			/src\/pages\/AgentsPage\//,
			/src\/pages\/AIBridgePage\//,
			/src\/pages\/TemplateBuilder\//,
		],
	},
};

// Serves the annotator bundle from the dev server so app previews can load
// it without a production build. Production gets it from `pnpm build`.
const annotatorDevServer = (): PluginOption => ({
	name: "coder-annotator-dev-server",
	apply: "serve",
	configureServer(server) {
		let bundle: Promise<string> | undefined;
		server.watcher.on("change", (file) => {
			if (file.includes(`${path.sep}src${path.sep}annotator${path.sep}`)) {
				bundle = undefined;
			}
		});
		server.middlewares.use("/annotator.js", async (_req, res, next) => {
			bundle ??= build({
				configFile: path.resolve(
					import.meta.dirname,
					"./vite.annotator.config.mts",
				),
				logLevel: "warn",
				build: { write: false },
			}).then((result) => {
				const outputs = Array.isArray(result) ? result : [result];
				for (const output of outputs) {
					if ("output" in output) {
						for (const chunk of output.output) {
							if (chunk.type === "chunk") {
								return chunk.code;
							}
						}
					}
				}
				throw new Error("annotator build produced no chunk");
			});
			try {
				const code = await bundle;
				res.setHeader("Content-Type", "text/javascript; charset=utf-8");
				res.end(code);
			} catch (error) {
				bundle = undefined;
				next(error);
			}
		});
	},
});

const coderHost = process.env.CODER_HOST || "http://localhost:3000";

// Coder routes subdomain workspace apps by Host header. App subdomains
// always have four "--" separated parts (port--agent--workspace--owner),
// which keeps dashed IPv6 hosts such as sslip.io names out of this path.
const isWorkspaceAppHost = (host: string | undefined): boolean =>
	(host ?? "").split(":")[0].split(".")[0].split("--").length >= 4;

// Forwards requests for workspace app subdomains to coderd untouched,
// Host header included, so port previews work when the wildcard access
// URL points at the dev server. Vite's built-in proxy matches on path
// only, so it cannot make this decision.
const workspaceAppProxy = (): PluginOption => ({
	name: "coder-workspace-app-proxy",
	apply: "serve",
	configureServer(server) {
		const target = new URL(coderHost);
		const secure = target.protocol === "https:";
		const port = Number(target.port || (secure ? 443 : 80));
		server.middlewares.use((req, res, next) => {
			if (!isWorkspaceAppHost(req.headers.host)) {
				next();
				return;
			}
			const upstream = (secure ? https : http).request(
				{
					host: target.hostname,
					port,
					method: req.method,
					path: req.url,
					headers: req.headers,
				},
				(upstreamRes) => {
					res.writeHead(upstreamRes.statusCode ?? 502, upstreamRes.headers);
					upstreamRes.pipe(res);
				},
			);
			upstream.on("error", (error) => {
				console.error(`workspace app proxy error: ${error.message}`);
				if (!res.headersSent) {
					res.writeHead(502, { "Content-Type": "text/plain" });
				}
				res.end();
			});
			req.pipe(upstream);
		});
		const proxyUpgrade = (
			req: http.IncomingMessage,
			socket: stream.Duplex,
			head: Buffer,
		) => {
			const connect = () => {
				const headerLines = [];
				for (let i = 0; i < req.rawHeaders.length; i += 2) {
					headerLines.push(`${req.rawHeaders[i]}: ${req.rawHeaders[i + 1]}`);
				}
				upstream.write(
					`${req.method} ${req.url} HTTP/${req.httpVersion}\r\n${headerLines.join("\r\n")}\r\n\r\n`,
				);
				upstream.write(head);
				socket.pipe(upstream).pipe(socket);
			};
			const upstream = secure
				? tls.connect(
						{ host: target.hostname, port, servername: target.hostname },
						connect,
					)
				: net.connect({ host: target.hostname, port }, connect);
			const close = () => {
				socket.destroy();
				upstream.destroy();
			};
			upstream.on("error", close);
			socket.on("error", close);
		};
		// Vite's HMR server claims every upgrade carrying the vite-hmr
		// protocol, Host header or not, which would hijack HMR sockets of
		// Vite apps running in a preview. Intercept the event before any
		// listener sees it instead of adding one more listener.
		const httpServer = server.httpServer;
		if (httpServer) {
			httpServer.emit = (event: string | symbol, ...args: unknown[]) => {
				if (event === "upgrade") {
					const [req, socket, head] = args;
					if (
						req instanceof http.IncomingMessage &&
						socket instanceof stream.Duplex &&
						Buffer.isBuffer(head) &&
						isWorkspaceAppHost(req.headers.host)
					) {
						proxyUpgrade(req, socket, head);
						return true;
					}
				}
				return EventEmitter.prototype.emit.call(httpServer, event, ...args);
			};
		}
	},
});

const plugins: PluginOption[] = [
	tailwindcss(),
	react(),
	babel({ presets: [compilerPreset] }),
	checker({
		typescript: true,
	}),
	annotatorDevServer(),
	workspaceAppProxy(),
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
	publicDir: path.resolve(import.meta.dirname, "./static"),
	build: {
		outDir: path.resolve(import.meta.dirname, "./out"),
		emptyOutDir: false, // We need to keep the /bin folder and GITKEEP files
		sourcemap: isProfilingBuild ? true : "hidden",
		rolldownOptions: {
			input: {
				index: path.resolve(import.meta.dirname, "./index.html"),
				serviceWorker: path.resolve(
					import.meta.dirname,
					"./src/serviceWorker.ts",
				),
			},
			output: {
				entryFileNames: (chunkInfo) => {
					return chunkInfo.name === "serviceWorker"
						? "[name].js"
						: "assets/[name]-[hash].js";
				},
				codeSplitting: {
					groups: [
						{ name: "monaco", test: /monaco-editor/ },
						{ name: "xterm", test: /@xterm/ },
						{ name: "emoji-mart", test: /emoji-mart/ },
						{ name: "radix-ui", test: /radix-ui/ },
					],
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
		// The proxy targets localhost:3000 (coderd). During tests no
		// coderd is running, and the proxy's retry sockets keep the
		// Node process alive after vitest finishes.
		proxy: process.env.VITEST
			? undefined
			: {
					"//": {
						changeOrigin: true,
						target: coderHost,
						secure: process.env.NODE_ENV === "production",
						rewrite: (path) => path.replace(/\/+/g, "/"),
					},
					"/api": {
						ws: true,
						changeOrigin: true,
						target: coderHost,
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
						target: coderHost,
						secure: process.env.NODE_ENV === "production",
					},
					"/healthz": {
						target: coderHost,
						secure: process.env.NODE_ENV === "production",
					},
					"/serviceWorker.js": {
						target: coderHost,
						secure: process.env.NODE_ENV === "production",
					},
				},
		// sslip.io gives wildcard DNS for a workspace IP, which is the
		// only way to reach subdomain apps through Coder Desktop.
		allowedHosts: [".coder", ".dogfood.cdr.dev", ".sslip.io"],
	},
	// Pre-bundle deps that Vite tends to discover late. Without this, Vite
	// re-optimizes mid-session which returns 504 "Outdated Optimize Dep" for
	// every previously served chunk, cascading into dynamic import failures.
	optimizeDeps: {
		include: [
			// Discovered at runtime without this entry, triggering
			// a mid-run dep re-optimization that breaks imports.
			"@tanstack/react-query-devtools",
		],
	},
	resolve: {
		alias: {
			// In profiling builds, swap the usual reconciler for the profiling
			// variant so that <Profiler> receives actual timing data.
			...(isProfilingBuild
				? { "react-dom/client": "react-dom/profiling" }
				: {}),
		},
	},
	test: {
		silent: "passed-only",
		// Rolldown's native threads do not terminate on close,
		// so vitest always hits this timeout. Keep it short.
		teardownTimeout: 1000,
		projects: [
			{
				extends: true,
				test: {
					name: "unit",
					include: [
						"src/**/*.test.?(m)ts?(x)",
						"scripts/**/*.test.?(m)[jt]s?(x)",
					],
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
						configDir: path.join(import.meta.dirname, ".storybook"),
					}),
					{
						name: "storybook-test-setup",
						// Return 502 for API routes. The proxy is disabled
						// during tests (see above), so without this vite
						// serves its HTML fallback for unmatched routes.
						configureServer(server) {
							server.middlewares.use((req, res, next) => {
								const url = req.url ?? "";
								if (
									url.startsWith("/api/") ||
									url.startsWith("/swagger/") ||
									url.startsWith("/healthz")
								) {
									res.statusCode = 502;
									res.end();
									return;
								}
								next();
							});
						},
					},
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
					// Stop early on systemic failures.
					bail: 5,
					// Cap concurrent browser iframes. The default
					// (os.availableParallelism, 96 on dev workspaces)
					// overwhelms vite's transform pipeline on cold cache.
					maxWorkers: 4,
				},
			},
		],
	},
});
