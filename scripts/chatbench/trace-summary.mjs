// Summarize a Chrome trace: main-thread time by event category and the
// longest tasks with their dominant child work.
//   node scripts/chatbench/trace-summary.mjs <trace.json> [topN]
import fs from "node:fs";

const file = process.argv[2];
const topN = Number(process.argv[3] ?? 8);
const { traceEvents } = JSON.parse(fs.readFileSync(file, "utf8"));

// Pick the renderer main thread: the CrRendererMain thread with the most
// FunctionCall/UpdateLayoutTree work.
const mainThreads = new Set(
	traceEvents
		.filter(
			(e) => e.name === "thread_name" && e.args?.name === "CrRendererMain",
		)
		.map((e) => `${e.pid}:${e.tid}`),
);
const byThread = new Map();
for (const e of traceEvents) {
	const k = `${e.pid}:${e.tid}`;
	if (
		mainThreads.has(k) &&
		(e.name === "FunctionCall" ||
			e.name === "UpdateLayoutTree" ||
			e.name === "Layout")
	) {
		byThread.set(k, (byThread.get(k) ?? 0) + (e.dur ?? 0));
	}
}
const [mainKey] = [...byThread.entries()].sort((a, b) => b[1] - a[1])[0] ?? [];
if (!mainKey)
	throw new Error("no renderer main thread work found; wrong categories?");
const [pid, tid] = mainKey.split(":").map(Number);
const main = traceEvents
	.filter((e) => e.pid === pid && e.tid === tid && e.ph === "X" && e.dur > 0)
	.sort((a, b) => a.ts - b.ts);

const interesting = new Set([
	"RunTask",
	"FunctionCall",
	"EventDispatch",
	"UpdateLayoutTree",
	"Layout",
	"PrePaint",
	"Paint",
	"Layerize",
	"HitTest",
	"TimerFire",
	"FireAnimationFrame",
	"RunMicrotasks",
	"ParseHTML",
	"MinorGC",
	"MajorGC",
	"V8.GC_TIME",
	"ScheduleStyleRecalculation",
	"InvalidateLayout",
	"Commit",
	"UpdateLayer",
	"ResourceReceivedData",
	"WebSocketReceive",
]);

// Self time per event name (exclusive of nested children).
const selfByName = new Map();
const stack = [];
for (const e of main) {
	while (
		stack.length &&
		stack[stack.length - 1].ts + stack[stack.length - 1].dur <= e.ts
	)
		stack.pop();
	const parent = stack[stack.length - 1];
	if (parent) parent.childDur = (parent.childDur ?? 0) + e.dur;
	stack.push(e);
}
for (const e of main) {
	const self = e.dur - (e.childDur ?? 0);
	selfByName.set(e.name, (selfByName.get(e.name) ?? 0) + self);
}
const total = [...selfByName.values()].reduce((a, b) => a + b, 0);
console.log(
	`main thread ${pid}:${tid}, ${main.length} events, ${(total / 1000).toFixed(0)}ms self time total`,
);
console.log("self time by event type:");
for (const [name, us] of [...selfByName.entries()]
	.sort((a, b) => b[1] - a[1])
	.slice(0, 14)) {
	console.log(`  ${(us / 1000).toFixed(0).padStart(7)}ms  ${name}`);
}

// Longest top-level tasks and what they contain.
const tasks = main.filter(
	(e) => e.name === "RunTask" || e.name === "ThreadControllerImpl::RunTask",
);
tasks.sort((a, b) => b.dur - a.dur);
console.log(`\nlongest tasks (of ${tasks.length}):`);
for (const t of tasks.slice(0, topN)) {
	const inside = main.filter(
		(e) =>
			e !== t &&
			e.ts >= t.ts &&
			e.ts + e.dur <= t.ts + t.dur &&
			interesting.has(e.name),
	);
	const agg = new Map();
	for (const e of inside)
		agg.set(e.name, (agg.get(e.name) ?? 0) + (e.dur - (e.childDur ?? 0)));
	const parts = [...agg.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 5)
		.map(([n, us]) => `${n} ${(us / 1000).toFixed(0)}ms`);
	const ev = inside.find((e) => e.name === "EventDispatch");
	const evName = ev?.args?.data?.type ? ` event=${ev.args.data.type}` : "";
	const layouts = inside.filter((e) => e.name === "Layout").length;
	const styles = inside.filter((e) => e.name === "UpdateLayoutTree");
	const styleNodes = styles.reduce(
		(a, e) => a + (e.args?.elementCount ?? e.args?.data?.elementCount ?? 0),
		0,
	);
	console.log(
		`  ${(t.dur / 1000).toFixed(0).padStart(6)}ms${evName} layouts=${layouts} styleRecalcs=${styles.length} styledElements=${styleNodes} :: ${parts.join(", ")}`,
	);
}
