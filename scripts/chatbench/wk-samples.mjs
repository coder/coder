// Aggregate the JS sampling-profiler samples in a WebKit Timeline
// recording saved by bench.mjs by source function, symbolicated through
// the production bundle's source maps in site/out/assets.
//
//   node scripts/chatbench/wk-samples.mjs <file.wktimeline.json> [self|inclusive|stacks] [depth]
//
// "self" (default) charges each sample to its innermost JS frame;
// "inclusive" charges it to every distinct frame on the stack;
// "stacks" ranks the innermost <depth> frames of each stack (default 6),
// keeping native builtins so forced layout reads stay visible.
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const REPO = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../..",
);
// trace-mapping is a transitive dependency of vite, not of site itself, so
// resolve it from vite's package under pnpm's strict layout.
const requireFromSite = createRequire(path.join(REPO, "site/package.json"));
const requireFromVite = createRequire(
	requireFromSite.resolve("vite/package.json"),
);
const { TraceMap, originalPositionFor } = requireFromVite(
	"@jridgewell/trace-mapping",
);

const [file, mode = "self", depthArg] = process.argv.slice(2);
const depth = Number.parseInt(depthArg ?? "", 10) || 6;
const d = JSON.parse(fs.readFileSync(file, "utf8"));

const maps = new Map();
const resolve = (fr) => {
	const m = (fr.url ?? "").match(/\/assets\/([^/?#]+\.js)$/);
	if (!m)
		return `${fr.name || "anon"} ${fr.url ? fr.url.split("/").pop() : "[native]"}`;
	const mapPath = path.join(REPO, "site/out/assets", `${m[1]}.map`);
	if (!maps.has(mapPath)) {
		maps.set(
			mapPath,
			fs.existsSync(mapPath)
				? new TraceMap(JSON.parse(fs.readFileSync(mapPath, "utf8")))
				: null,
		);
	}
	const tm = maps.get(mapPath);
	if (!tm) return `${fr.name || "anon"} ${m[1]}:${fr.line}:${fr.column}`;
	// WebKit call frames are 1-based lines and 1-based columns.
	const pos = originalPositionFor(tm, {
		line: fr.line,
		column: Math.max(0, fr.column - 1),
	});
	if (!pos.source)
		return `${fr.name || "anon"} ${m[1]}:${fr.line}:${fr.column}`;
	const src = pos.source
		.replace(/^.*?\/site\//, "")
		.replace(/^(\.\.\/)+/, "")
		.replace(/^node_modules\/\.pnpm\/([^/]+)\/node_modules\//, "[$1] ");
	return `${pos.name ?? fr.name ?? "anon"} ${src}:${pos.line}`;
};

const agg = new Map();
for (const s of d.samples ?? []) {
	const frames = s.stackFrames ?? [];
	if (frames.length === 0) continue;
	if (mode === "stacks") {
		const k = frames.slice(0, depth).map(resolve).join("\n        < ");
		agg.set(k, (agg.get(k) ?? 0) + 1);
		continue;
	}
	if (mode === "self") {
		// Skip native builtins so the sample lands on the calling JS.
		const top = frames.find((f) => f.url) ?? frames[0];
		const k = resolve(top);
		agg.set(k, (agg.get(k) ?? 0) + 1);
		continue;
	}
	const seen = new Set();
	for (const f of frames) {
		const k = resolve(f);
		if (seen.has(k)) continue;
		seen.add(k);
		agg.set(k, (agg.get(k) ?? 0) + 1);
	}
}
const total = (d.samples ?? []).length;
console.log(`${total} samples (${mode})`);
for (const [k, n] of [...agg.entries()]
	.sort((a, b) => b[1] - a[1])
	.slice(0, mode === "stacks" ? 12 : 30)) {
	console.log(
		`${String(n).padStart(6)} ${((n / total) * 100).toFixed(1).padStart(5)}%  ${k}`,
	);
}
