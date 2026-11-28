// Dissect long frames in a WebKit Timeline recording saved by bench.mjs:
// the record tree inside each frame (with JS stacks where WebKit
// captured them), the top-level records that preceded it, and the JS
// sampling-profiler frames that were on the stack around it.
//
//   node scripts/chatbench/wk-frame.mjs <file.wktimeline.json> [minFrameMs] [beforeMs]
//
// minFrameMs selects frames to dissect (default 1000). beforeMs prints
// every record in that window before each frame in detail (default 0).
import fs from "node:fs";

const [file, minMsArg = "1000", beforeMsArg = "0"] = process.argv.slice(2);
const minMs = Number(minMsArg);
const beforeMs = Number(beforeMsArg);
const d = JSON.parse(fs.readFileSync(file, "utf8"));
const dur = (r) => ((r.endTime ?? r.startTime) - r.startTime) * 1000;
const recs = d.records.sort((a, b) => a.startTime - b.startTime);
const label = (r) => r.type + (r.data?.type ? `(${r.data.type})` : "");
const frameOf = (fr) =>
	`${fr.functionName || "anon"}${fr.url ? `@${fr.url.split("/").pop()}:${fr.lineNumber}:${fr.columnNumber}` : ""}`;
const stackOf = (r) =>
	(r.stackTrace?.callFrames ?? []).slice(0, 5).map(frameOf).join(" < ");

const show = (r, origin, depth, maxDepth) => {
	if (depth > maxDepth) return;
	if (dur(r) < 1 && depth > 0 && beforeMs === 0) return;
	let childDur = 0;
	for (const c of r.children ?? []) childDur += dur(c);
	const at = `${((r.startTime - origin) * 1000).toFixed(1)}ms`;
	const fc =
		r.type === "FunctionCall" && r.data
			? ` ${String(r.data.scriptName).split("/").pop()}:${r.data.scriptLine}:${r.data.scriptColumn}`
			: "";
	const data =
		r.data && r.type !== "EventDispatch" && r.type !== "FunctionCall"
			? ` ${JSON.stringify(r.data).slice(0, 120)}`
			: "";
	const stack = r.stackTrace ? `  [${stackOf(r)}]` : "";
	console.log(
		`${"  ".repeat(depth)}${at} ${label(r)} ${dur(r).toFixed(1)}ms self=${(dur(r) - childDur).toFixed(1)}${fc}${data}${stack}`,
	);
	for (const c of r.children ?? []) show(c, origin, depth + 1, maxDepth);
};

const big = recs.filter((r) => dur(r) >= minMs);
console.log(
	`${recs.length} top-level records; ${big.length} frames >= ${minMs}ms: ${big.map((f) => `${f.startTime.toFixed(2)}s ${dur(f).toFixed(0)}ms`).join(", ")}`,
);

for (const f of big) {
	console.log(
		`\n=== frame at ${f.startTime.toFixed(3)}s, ${dur(f).toFixed(0)}ms (${f.data?.name ?? ""})`,
	);
	show(f, f.startTime, 0, 4);

	if (beforeMs > 0) {
		console.log(
			`-- records in the ${beforeMs}ms before the frame (offsets relative to frame start):`,
		);
		for (const r of recs.filter(
			(r) =>
				r.startTime >= f.startTime - beforeMs / 1000 &&
				r.startTime < f.startTime,
		))
			show(r, f.startTime, 0, 6);
	}

	const samples = (d.samples ?? []).filter(
		(s) =>
			s.timestamp >= f.startTime - 0.3 &&
			s.timestamp <= (f.endTime ?? f.startTime),
	);
	const tops = new Map();
	for (const s of samples) {
		const fr = s.stackFrames?.find((x) => x.url) ?? s.stackFrames?.[0];
		const k = fr
			? `${fr.name || "anon"} ${(fr.url || "").split("/").pop()}:${fr.line}:${fr.column}`
			: "(no frames)";
		tops.set(k, (tops.get(k) ?? 0) + 1);
	}
	console.log(
		`-- ${samples.length} JS samples from 300ms before the frame to its end${samples.length ? "; top frames:" : " (no JS was running)"}`,
	);
	for (const [k, n] of [...tops.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 8))
		console.log(`  ${n} ${k}`);
}
