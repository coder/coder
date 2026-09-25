import path from "node:path";
import { defineConfig, type Plugin } from "vite";

const annotatorDir = path.resolve(import.meta.dirname, "src/annotator");

/**
 * The overlay is injected into third-party pages as a classic
 * `<script src>`, so it must not rely on anything those pages do not
 * have: no framework, no shared chunks, no packages. Every import under
 * src/annotator is therefore relative, and this plugin fails the build
 * on any that is not, before the resolver can find it in node_modules.
 */
function dependencyFree(): Plugin {
	return {
		name: "coder:annotator-dependency-free",
		enforce: "pre",
		resolveId(source, importer) {
			const external =
				importer !== undefined &&
				!source.startsWith(".") &&
				!path.isAbsolute(source) &&
				!source.startsWith("\0");
			if (external) {
				this.error(
					`${path.relative(annotatorDir, importer)} imports "${source}". ` +
						"The annotator runs inside pages we do not control and must " +
						"stay dependency-free; only relative imports are allowed.",
				);
			}
			return null;
		},
	};
}

// Builds `out/annotator.js` alongside the dashboard build, which the
// Go binary embeds and serves at /annotator.js (see site.go for the
// cache rule that keeps it in step with the dashboard it shipped with).
export default defineConfig({
	publicDir: false,
	plugins: [dependencyFree()],
	build: {
		outDir: path.resolve(import.meta.dirname, "out"),
		emptyOutDir: false,
		sourcemap: "hidden",
		lib: {
			entry: path.join(annotatorDir, "main.ts"),
			formats: ["iife"],
			name: "CoderAnnotator",
			fileName: () => "annotator.js",
		},
	},
});
