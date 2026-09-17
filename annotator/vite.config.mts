import { defineConfig } from "vite";

// The annotator is injected into proxied workspace apps as a classic
// `<script src>`, so it must be a single self-contained IIFE with no
// external chunks. The Makefile copies dist/annotator.js into site/out.
export default defineConfig({
	resolve: {
		alias: {
			react: new URL("./src/reactShim.ts", import.meta.url).pathname,
		},
	},
	publicDir: false,
	build: {
		outDir: "dist",
		sourcemap: "hidden",
		lib: {
			entry: "src/main.ts",
			formats: ["iife"],
			name: "CoderAnnotator",
			fileName: () => "annotator.js",
		},
	},
});
