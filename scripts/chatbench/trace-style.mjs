// Explain style recalculation in a Chrome trace recorded with the
// devtools.timeline.stack and invalidationTracking categories:
//   - which JS scheduled (dirtied) style before each full-document recalc
//   - which JS forced the recalc synchronously (layout read)
//   - the invalidation reasons Blink recorded (node, reason, selector)
// symbolicated through the production bundle's hidden source maps.
//   node scripts/chatbench/trace-style.mjs <trace.json> [topN]
import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { traceEventsOf } from "./trace-stream.mjs";

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
const topN = Number(process.argv[3] ?? 3);

const maps = new Map();
function resolve(frame) {
	const url = frame.url ?? "";
	const fn = frame.functionName || "(anon)";
	const m = url.match(/\/assets\/([^/?#]+\.js)$/);
	if (!m) return `${fn} ${url}:${frame.lineNumber}:${frame.columnNumber}`;
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
	const pos = originalPositionFor(tm, {
		line: frame.lineNumber ?? 1,
		column: frame.columnNumber ?? 0,
	});
	if (!pos.source)
		return `${fn} ${m[1]}:${frame.lineNumber}:${frame.columnNumber}`;
	const src = pos.source.replace(/^.*?\/site\//, "").replace(/^(\.\.\/)+/, "");
	return `${pos.name ?? fn} ${src}:${pos.line}:${pos.column}`;
}
const isApp = (f) =>
	/\/assets\//.test(f.url ?? "") && !/react-dom|vendor|radix/.test(f.url ?? "");
const appTop = (st) => {
	const top = (st ?? []).find(isApp) ?? (st ?? [])[0];
	return top ? resolve(top) : "(no JS stack)";
};

const wanted = new Set([
	"thread_name",
	"UpdateLayoutTree",
	"ScheduleStyleRecalculation",
	"StyleRecalcInvalidationTracking",
	"ScheduleStyleInvalidationTracking",
	"StyleInvalidatorInvalidationTracking",
	"EventDispatch",
]);

const mainThreads = new Set();
const recalcs = [];
const schedules = [];
const invalidations = [];
let invalidationCount = 0;
const invalidationReasons = new Map();
const invalidationStacks = new Map();
for await (const e of traceEventsOf(file, (ev) => wanted.has(ev.name))) {
	if (e.name === "thread_name") {
		if (e.args?.name === "CrRendererMain") mainThreads.add(`${e.pid}:${e.tid}`);
		continue;
	}
	if (!mainThreads.has(`${e.pid}:${e.tid}`)) continue;
	if (e.name === "UpdateLayoutTree" && e.ph === "X") recalcs.push(e);
	else if (e.name === "ScheduleStyleRecalculation") schedules.push(e);
	else if (e.name.endsWith("InvalidationTracking")) {
		invalidationCount++;
		const d = e.args?.data ?? {};
		const reason = `${e.name.replace("InvalidationTracking", "")}: ${d.reason ?? "?"}${d.invalidatedSelectorId ? ` (${d.invalidatedSelectorId})` : ""}${d.extraData ? ` ${d.extraData}` : ""}${d.selectorPart ? ` ${d.selectorPart}` : ""}`;
		invalidationReasons.set(reason, (invalidationReasons.get(reason) ?? 0) + 1);
		if (d.stackTrace?.length) {
			const k = `${reason} <- ${appTop(d.stackTrace)}`;
			invalidationStacks.set(k, (invalidationStacks.get(k) ?? 0) + 1);
		}
		if (
			invalidations.length < 20 &&
			d.stackTrace?.length &&
			(d.nodeName ?? "").toLowerCase().startsWith("html")
		) {
			invalidations.push({
				reason,
				node: d.nodeName,
				stack: d.stackTrace.slice(0, 6).map(resolve),
			});
		}
	}
}
recalcs.sort((a, b) => b.dur - a.dur);
schedules.sort((a, b) => a.ts - b.ts);

const total = recalcs.reduce((a, e) => a + e.dur, 0);
const big = recalcs.filter((e) => (e.args?.elementCount ?? 0) > 10000);
console.log(
	`${recalcs.length} UpdateLayoutTree events totaling ${(total / 1000).toFixed(0)}ms`,
);
console.log(
	`${big.length} recalcs styled >10k elements each, accounting for ${(big.reduce((a, e) => a + e.dur, 0) / 1000).toFixed(0)}ms`,
);
console.log(`${invalidationCount} invalidation-tracking events`);

const lastScheduleBefore = (ts) => {
	let lo = 0;
	let hi = schedules.length - 1;
	let ans = null;
	while (lo <= hi) {
		const mid = (lo + hi) >> 1;
		if (schedules[mid].ts <= ts) {
			ans = schedules[mid];
			lo = mid + 1;
		} else hi = mid - 1;
	}
	return ans;
};

const forcedBy = new Map();
const scheduledBy = new Map();
for (const r of big) {
	const f = appTop(r.args?.beginData?.stackTrace);
	forcedBy.set(f, (forcedBy.get(f) ?? 0) + r.dur);
	const s = lastScheduleBefore(r.ts);
	const k = appTop(s?.args?.data?.stackTrace);
	scheduledBy.set(k, (scheduledBy.get(k) ?? 0) + r.dur);
}
const printMap = (title, m, fmt) => {
	console.log(`\n${title}`);
	for (const [k, v] of [...m.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 10))
		console.log(`  ${fmt(v).padStart(8)}  ${k}`);
};
printMap(
	"full-document recalcs by JS frame that SCHEDULED (dirtied) style:",
	scheduledBy,
	(us) => `${(us / 1000).toFixed(0)}ms`,
);
printMap(
	"full-document recalcs by JS frame that FORCED the recalc (sync layout read):",
	forcedBy,
	(us) => `${(us / 1000).toFixed(0)}ms`,
);
printMap(
	"invalidation reasons (count of invalidated nodes):",
	invalidationReasons,
	(n) => String(n),
);
printMap(
	"invalidation reasons with the JS stack that caused them:",
	invalidationStacks,
	(n) => String(n),
);

console.log(
	`\nroot-element invalidations with stacks (first ${invalidations.length}):`,
);
for (const inv of invalidations.slice(0, 4)) {
	console.log(`  ${inv.node}: ${inv.reason}`);
	for (const s of inv.stack) console.log(`      ${s}`);
}

console.log(`\nlargest ${topN} recalcs:`);
for (const r of recalcs.slice(0, topN)) {
	console.log(
		`\n-- ${(r.dur / 1000).toFixed(0)}ms, ${r.args?.elementCount} elements`,
	);
	console.log("  forced by:");
	for (const f of (r.args?.beginData?.stackTrace ?? []).slice(0, 7))
		console.log(`    ${resolve(f)}`);
	const s = lastScheduleBefore(r.ts);
	console.log("  scheduled by:");
	for (const f of (s?.args?.data?.stackTrace ?? []).slice(0, 7))
		console.log(`    ${resolve(f)}`);
}
