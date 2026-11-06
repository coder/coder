// Read React's own performance tracks out of a Chromium trace recorded
// against a CODER_REACT_PROFILING=true build. react-dom/profiling logs
// every render phase ("Scheduler ⚛": Event, Update, Render, Commit, ...)
// and every component render/effect ("Components ⚛") through
// console.timeStamp, which Chromium records as TimeStamp trace events.
// This prints, per long main-thread task: the browser work breakdown, the
// React scheduler entries inside it, the components with the most self
// time, and the DOM node counter before and after (a drop and rise in
// nodes between commits means a subtree was remounted).
//
//   node scripts/chatbench/trace-react.mjs <trace.json> [minTaskMs] [topN]
import { traceEventsOf } from "./trace-stream.mjs";

const file = process.argv[2];
const minTaskMs = Number(process.argv[3] ?? 50);
const topN = Number(process.argv[4] ?? 8);

const KEEP = new Set([
	"thread_name",
	"RunTask",
	"TimeStamp",
	"UpdateCounters",
	"UpdateLayoutTree",
	"Layout",
	"FunctionCall",
	"EventDispatch",
	"EventTiming",
	"Paint",
	"PrePaint",
	"Commit",
	"Layerize",
	"HitTest",
	"MajorGC",
	"MinorGC",
	"V8.GC_MC_INCREMENTAL",
]);

const events = [];
for await (const e of traceEventsOf(file, (e) => KEEP.has(e.name)))
	events.push(e);

const mainThreads = new Set(
	events
		.filter(
			(e) => e.name === "thread_name" && e.args?.name === "CrRendererMain",
		)
		.map((e) => `${e.pid}:${e.tid}`),
);
const work = new Map();
for (const e of events) {
	const k = `${e.pid}:${e.tid}`;
	if (mainThreads.has(k) && e.ph === "X" && e.name !== "RunTask")
		work.set(k, (work.get(k) ?? 0) + (e.dur ?? 0));
}
const [mainKey] = [...work.entries()].sort((a, b) => b[1] - a[1])[0] ?? [];
if (!mainKey) throw new Error("no renderer main thread work found");
const [pid, tid] = mainKey.split(":").map(Number);
const main = events.filter((e) => e.pid === pid && e.tid === tid);

const tasks = main
	.filter(
		(e) => e.name === "RunTask" && e.ph === "X" && e.dur >= minTaskMs * 1000,
	)
	.sort((a, b) => a.ts - b.ts);
const spans = main.filter((e) => e.ph === "X" && e.name !== "RunTask");
const react = main.filter(
	(e) => e.name === "TimeStamp" && e.args?.data?.trackGroup,
);
const counters = main
	.filter((e) => e.name === "UpdateCounters")
	.sort((a, b) => a.ts - b.ts);
const t0 = Math.min(...main.map((e) => e.ts));
const ms = (us) => (us / 1000).toFixed(1);
const at = (us) => `+${ms(us - t0)}ms`;

// Whole-recording React totals.
const compTotal = new Map();
const schedTotal = new Map();
for (const e of react) {
	const d = e.args.data;
	const dur = d.end - d.start;
	if (d.trackGroup === "Components ⚛") {
		const c = compTotal.get(d.name) ?? { n: 0, us: 0, max: 0 };
		c.n++;
		c.us += dur;
		c.max = Math.max(c.max, dur);
		compTotal.set(d.name, c);
	} else if (d.trackGroup === "Scheduler ⚛") {
		const k = `${d.track}/${d.name}`;
		const c = schedTotal.get(k) ?? { n: 0, us: 0, max: 0 };
		c.n++;
		c.us += dur;
		c.max = Math.max(c.max, dur);
		schedTotal.set(k, c);
	}
}
const table = (m, n = topN) =>
	[...m.entries()]
		.sort((a, b) => b[1].us - a[1].us)
		.slice(0, n)
		.map(
			([k, c]) =>
				`    ${ms(c.us).padStart(9)}ms  n=${String(c.n).padStart(6)}  max=${ms(c.max).padStart(8)}ms  ${k}`,
		)
		.join("\n");

console.log(
	`${file}\nmain thread ${pid}:${tid}, ${react.length} React track entries, ${tasks.length} tasks >= ${minTaskMs}ms`,
);
console.log("\nScheduler ⚛ totals (track/entry):\n" + table(schedTotal, 20));
console.log(
	"\nComponents ⚛ totals by component (self time as logged by React):\n" +
		table(compTotal, 25),
);

if (counters.length > 1) {
	const nodes = counters.map((c) => c.args.data.nodes);
	console.log(
		`\nDOM nodes counter: min ${Math.min(...nodes)} max ${Math.max(...nodes)} first ${nodes[0]} last ${nodes[nodes.length - 1]}`,
	);
	// Largest swings between consecutive samples.
	const swings = [];
	for (let i = 1; i < counters.length; i++) {
		const d = counters[i].args.data.nodes - counters[i - 1].args.data.nodes;
		if (Math.abs(d) > 2000)
			swings.push(`${at(counters[i].ts)} ${d > 0 ? "+" : ""}${d}`);
	}
	if (swings.length)
		console.log(
			"  swings > 2000 nodes: " +
				swings.slice(0, 20).join(", ") +
				(swings.length > 20 ? ` ... (${swings.length})` : ""),
		);
}

for (const task of tasks) {
	const end = task.ts + task.dur;
	const inside = (e) => e.ts >= task.ts && e.ts < end;
	const bySpan = new Map();
	for (const s of spans.filter(inside))
		bySpan.set(s.name, (bySpan.get(s.name) ?? 0) + s.dur);
	const breakdown = [...bySpan.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 6)
		.map(([n, us]) => `${n} ${ms(us)}ms`)
		.join(", ");
	const ev = spans
		.filter((e) => inside(e) && e.name === "EventDispatch")
		.map((e) => e.args?.data?.type);
	console.log(
		`\n=== task ${at(task.ts)} ${ms(task.dur)}ms  events=[${[...new Set(ev)].join(",")}]\n  browser: ${breakdown}`,
	);
	// React entries are logged at commit time with their own start/end, so
	// match by their time range instead of the log timestamp.
	const inTask = react.filter(
		(e) => e.args.data.end > task.ts && e.args.data.start < end,
	);
	const sched = inTask
		.filter((e) => e.args.data.trackGroup === "Scheduler ⚛")
		.sort((a, b) => a.args.data.start - b.args.data.start)
		.map(
			(e) =>
				`${e.args.data.track}/${e.args.data.name} ${ms(e.args.data.end - e.args.data.start)}ms`,
		);
	if (sched.length)
		console.log(
			`  react: ${sched.slice(0, 12).join(" -> ")}${sched.length > 12 ? ` ... (+${sched.length - 12})` : ""}`,
		);
	const comps = new Map();
	for (const e of inTask.filter(
		(e) => e.args.data.trackGroup === "Components ⚛",
	)) {
		const d = e.args.data;
		const c = comps.get(d.name) ?? { n: 0, us: 0, max: 0 };
		c.n++;
		c.us += d.end - d.start;
		c.max = Math.max(c.max, d.end - d.start);
		comps.set(d.name, c);
	}
	if (comps.size) {
		const total = [...comps.values()].reduce((a, c) => a + c.us, 0);
		const n = [...comps.values()].reduce((a, c) => a + c.n, 0);
		console.log(
			`  components: ${n} renders, ${ms(total)}ms logged self time\n${table(comps)}`,
		);
	}
	const before = counters.filter((c) => c.ts <= task.ts).pop();
	const after = counters.find((c) => c.ts >= end);
	if (before && after)
		console.log(
			`  nodes: ${before.args.data.nodes} -> ${after.args.data.nodes}, listeners ${before.args.data.jsEventListeners} -> ${after.args.data.jsEventListeners}`,
		);
}
