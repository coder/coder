// Summarize a WebKit Timeline recording saved by bench.mjs: self time by
// record type, the longest top-level records with their breakdown, and
// the JS stacks that scheduled or forced style recalcs and layouts,
// symbolicated through the production bundle's hidden source maps.
//   node scripts/chatbench/wk-summary.mjs <file.wktimeline.json> [topN]
import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const REPO = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../..",
);
// trace-mapping is a transitive dependency of vite, not of site itself, so
// resolve it from vite's package under pnpm's strict layout.
const requireFromSite = createRequire(path.join(REPO, "site/package.json"));
const requireFromVite = createRequire(requireFromSite.resolve("vite/package.json"));
const { TraceMap, originalPositionFor } = requireFromVite("@jridgewell/trace-mapping");

const file = process.argv[2];
const topN = Number(process.argv[3] ?? 6);
const { records } = JSON.parse(fs.readFileSync(file, "utf8"));

const maps = new Map();
function resolve(frame) {
	const url = frame.url ?? "";
	const fn = frame.functionName || "(anon)";
	const m = url.match(/\/assets\/([^/?#]+\.js)$/);
	if (!m)
		return `${fn} ${url || "[native]"}:${frame.lineNumber}:${frame.columnNumber}`;
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
	if (!tm) return `${fn} ${m[1]}:${frame.lineNumber}:${frame.columnNumber}`;
	// WebKit call frames are 1-based lines and 1-based columns.
	const pos = originalPositionFor(tm, {
		line: frame.lineNumber,
		column: Math.max(0, frame.columnNumber - 1),
	});
	if (!pos.source)
		return `${fn} ${m[1]}:${frame.lineNumber}:${frame.columnNumber}`;
	const src = pos.source.replace(/^.*?\/site\//, "").replace(/^(\.\.\/)+/, "");
	return `${pos.name ?? fn} ${src}:${pos.line}:${pos.column}`;
}
const isApp = (f) =>
	/\/assets\//.test(f.url ?? "") && !/react-dom|vendor|radix/.test(f.url ?? "");
const appTop = (frames) => {
	const top =
		(frames ?? []).find(isApp) ??
		(frames ?? []).find((f) => f.url) ??
		(frames ?? [])[0];
	return top ? resolve(top) : "(no JS stack)";
};

const dur = (r) => ((r.endTime ?? r.startTime) - r.startTime) * 1000;
const selfByType = new Map();
const scheduledBy = new Map();
const forcedBy = new Map();
const layoutBy = new Map();
let lastSchedule = null;
const walk = (r) => {
	let childDur = 0;
	for (const c of r.children ?? []) {
		childDur += dur(c);
		walk(c);
	}
	const self = Math.max(0, dur(r) - childDur);
	selfByType.set(r.type, (selfByType.get(r.type) ?? 0) + self);
	if (r.type === "ScheduleStyleRecalculation") lastSchedule = r;
	if (r.type === "RecalculateStyles" && dur(r) > 20) {
		const f = appTop(r.stackTrace?.callFrames);
		forcedBy.set(f, (forcedBy.get(f) ?? 0) + dur(r));
		const s = appTop(lastSchedule?.stackTrace?.callFrames);
		scheduledBy.set(s, (scheduledBy.get(s) ?? 0) + dur(r));
	}
	if (r.type === "Layout" && dur(r) > 20) {
		const f = appTop(r.stackTrace?.callFrames);
		layoutBy.set(f, (layoutBy.get(f) ?? 0) + dur(r));
	}
};
// Records arrive in time order; walk in order so lastSchedule is meaningful.
records.sort((a, b) => a.startTime - b.startTime);
for (const r of records) walk(r);

const total = [...selfByType.values()].reduce((a, b) => a + b, 0);
console.log(
	`${records.length} top-level records, ${total.toFixed(0)}ms recorded`,
);
console.log("self time by record type:");
for (const [t, ms] of [...selfByType.entries()]
	.sort((a, b) => b[1] - a[1])
	.slice(0, 12)) {
	console.log(`  ${ms.toFixed(0).padStart(7)}ms  ${t}`);
}
const printMap = (title, m) => {
	console.log(`\n${title}`);
	for (const [k, ms] of [...m.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 8))
		console.log(`  ${ms.toFixed(0).padStart(7)}ms  ${k}`);
};
printMap("style recalcs >20ms by JS frame that SCHEDULED them:", scheduledBy);
printMap(
	"style recalcs >20ms by JS frame that FORCED them (sync read):",
	forcedBy,
);
printMap("layouts >20ms by JS frame that forced them:", layoutBy);

console.log(`\nlongest ${topN} top-level records:`);
const top = [...records].sort((a, b) => dur(b) - dur(a)).slice(0, topN);
for (const r of top) {
	const agg = new Map();
	const flat = [];
	const collect = (x, depth) => {
		flat.push({ x, depth });
		for (const c of x.children ?? []) collect(c, depth + 1);
	};
	collect(r, 0);
	for (const { x } of flat) {
		let childDur = 0;
		for (const c of x.children ?? []) childDur += dur(c);
		agg.set(x.type, (agg.get(x.type) ?? 0) + Math.max(0, dur(x) - childDur));
	}
	const parts = [...agg.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 5)
		.map(([t, ms]) => `${t} ${ms.toFixed(0)}ms`);
	const ev = flat.find(({ x }) => x.type === "EventDispatch");
	const evName = ev ? ` event=${ev.x.data?.type}` : "";
	const layouts = flat.filter(({ x }) => x.type === "Layout");
	const totalObjects = layouts.reduce(
		(a, { x }) => a + (x.data?.totalObjects ?? 0),
		0,
	);
	const dirtyObjects = layouts.reduce(
		(a, { x }) => a + (x.data?.dirtyObjects ?? 0),
		0,
	);
	console.log(
		`\n-- ${dur(r).toFixed(0)}ms ${r.type}${evName} :: ${parts.join(", ")} | layouts=${layouts.length} dirty/total objects=${dirtyObjects}/${totalObjects}`,
	);
	const heavy = flat
		.filter(
			({ x }) =>
				(x.type === "RecalculateStyles" ||
					x.type === "Layout" ||
					x.type === "FunctionCall") &&
				dur(x) > 30,
		)
		.slice(0, 4);
	for (const { x } of heavy) {
		console.log(
			`   ${x.type} ${dur(x).toFixed(0)}ms ${x.data?.dirtyObjects !== undefined ? `dirty=${x.data.dirtyObjects}/${x.data.totalObjects}` : ""}`,
		);
		for (const f of (x.stackTrace?.callFrames ?? []).slice(0, 6))
			console.log(`      ${resolve(f)}`);
	}
}
