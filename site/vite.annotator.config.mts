import path from "node:path";
import { build, defineConfig, type Plugin } from "vite";

const annotatorDir = path.resolve(import.meta.dirname, "src/annotator");

// The classic script runs in third-party pages without the dashboard's
// framework, package imports, or shared chunks.
function dependencyFree(): Plugin {
	return {
		name: "coder:annotator-dependency-free",
		enforce: "pre",
		resolveId(source, importer) {
			if (importer && !source.startsWith(".") && !source.startsWith("\0")) {
				this.error(
					`${path.relative(annotatorDir, importer)} imports "${source}". ` +
						"The annotator must use relative imports only.",
				);
			}
			return null;
		},
	};
}

/** Serve the same classic-script bundle from the Vite development dashboard. */
export function annotatorDevServer(): Plugin {
	return {
		name: "coder:annotator-dev-server",
		apply: "serve",
		configureServer(server) {
			server.middlewares.use(async (request, response, next) => {
				if (request.url?.split("?")[0] !== "/annotator.js") {
					next();
					return;
				}
				if (request.method !== "GET" && request.method !== "HEAD") {
					response.statusCode = 405;
					response.setHeader("Allow", "GET, HEAD");
					response.end();
					return;
				}
				try {
					const result = await build({
						configFile: path.resolve(
							import.meta.dirname,
							"vite.annotator.config.mts",
						),
						logLevel: "silent",
						build: { write: false },
					});
					const bundles = Array.isArray(result) ? result : [result];
					const script = bundles
						.flatMap((bundle) => ("output" in bundle ? bundle.output : []))
						.find((file) => file.fileName === "annotator.js");
					if (script?.type !== "chunk") {
						throw new Error("Annotator build did not emit annotator.js");
					}
					response.setHeader("Content-Type", "text/javascript; charset=utf-8");
					response.setHeader("Cache-Control", "no-store");
					response.end(request.method === "HEAD" ? undefined : script.code);
				} catch (error) {
					server.config.logger.error(
						`Failed to build annotator.js: ${String(error)}`,
					);
					response.statusCode = 500;
					response.end("Unable to build annotator.js");
				}
			});
		},
	};
}

export default defineConfig({
	publicDir: false,
	plugins: [dependencyFree()],
	build: {
		outDir: path.resolve(import.meta.dirname, "out"),
		// Preserve the dashboard and binaries embedded from the same directory.
		emptyOutDir: false,
		lib: {
			entry: path.join(annotatorDir, "main.ts"),
			formats: ["iife"],
			name: "CoderAnnotator",
			fileName: () => "annotator.js",
		},
	},
});
