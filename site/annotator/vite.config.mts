import * as path from "node:path";
import { defineConfig } from "vite";

// The annotator is injected into proxied workspace apps as a classic
// `<script src>`, so it must be a single self-contained IIFE. The main
// build would otherwise hoist modules it shares with the dashboard (the
// postMessage protocol) into a separate chunk and emit an ES module.
export default defineConfig({
	publicDir: false,
	build: {
		outDir: path.resolve(import.meta.dirname, "../out"),
		emptyOutDir: false,
		sourcemap: "hidden",
		lib: {
			entry: path.resolve(import.meta.dirname, "../src/annotator/main.ts"),
			formats: ["iife"],
			name: "CoderAnnotator",
			fileName: () => "annotator.js",
		},
	},
});
