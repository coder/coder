// Main-thread attribution by task type using a CDP trace.
//
// The V8 CPU profile attributes JS functions but lumps all browser-internal
// work into "(program)". This script classifies main-thread time into the
// categories that matter for render cost: script evaluation, style
// recalculation, layout, paint, HTML parsing and GC. That distinction is
// what decides whether the fix belongs in application code or in CSS.
//
// Usage: node scripts/perf/trace-chat.mjs <scenario> <phase> [out.json]
import { chromium } from "@playwright/test";
import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const siteRoot = path.resolve(import.meta.dirname, "../..");
const outDir = path.join(siteRoot, "out");
const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const scenario = process.argv[2] ?? "turns=100&panels=4&streamChars=0";
const phase = process.argv[3] ?? "mount";
const outFile = process.argv[4] ?? "/tmp/trace-chat.json";

// Trace events that represent work on the main thread. Durations are
// summed per name so the report can rank them.
const CATEGORY_HINTS = [
	["ScriptEvaluation", "javascript"],
	["FunctionCall", "javascript"],
	["V8.Execute", "javascript"],
	["UpdateLayoutTree", "style"],
	["RecalcStyle", "style"],
	["Layout", "layout"],
	["Paint", "paint"],
	["PaintImage", "paint"],
	["ParseHTML", "html"],
	["ParseAuthorStyleSheet", "html"],
	["MajorGC", "gc"],
	["MinorGC", "gc"],
	["BlinkGC", "gc"],
	["Commit", "composite"],
	["CompositeLayers", "composite"],
	["RunTask", "task"],
	["TimerFire", "timer"],
	["EventDispatch", "event"],
	["ResourceSendRequest", "network"],
];

const serverReady = async () => {
	try {
		const res = await fetch(`http://127.0.0.1:${PORT}${BENCH_PATH}`, {
			method: "HEAD",
		});
		return res.ok;
	} catch {
		return false;
	}
};

const startServer = () =>
	new Promise((resolve, reject) => {
		const child = spawn(
			process.execPath,
			[path.join(siteRoot, "scripts/perf/chat-server.mjs")],
			{
				env: {
					...process.env,
					PERF_WEB_PORT: String(PORT),
					PERF_WEB_ROOT: outDir,
				},
				stdio: ["ignore", "pipe", "pipe"],
			},
		);
		child.stdout.on("data", (chunk) => {
			if (chunk.toString().includes("serving")) resolve(child);
		});
		child.on("error", reject);
		setTimeout(() => reject(new Error("bench server did not start")), 10_000);
	});

const ensureServer = async () => ((await serverReady()) ? null : startServer());

const main = async () => {
	const server = await ensureServer();
	const browser = await chromium.launch();
	const context = await browser.newContext({ viewport: { width: 1512, height: 950 } });
	const page = await context.newPage();
	const cdp = await context.newCDPSession(page);
	await cdp.send("Tracing.start", {
		traceConfig: {
			includedCategories: [
				"devtools.timeline",
				"v8.execute",
				"blink.user_timing",
			],
		},
	});

	let events = [];
	cdp.on("Tracing.dataCollected", (e) => {
		events = events.concat(e.value ?? []);
	});
	const done = new Promise((resolve) => {
		cdp.once("Tracing.tracingComplete", resolve);
	});

	// EXTRA appends experiment query params, e.g. EXTRA=perfCv=1, so one
	// trace can be compared against another for the same scenario.
	const extra = process.env.EXTRA ? `&${process.env.EXTRA}` : "";
	await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${scenario}${extra}`, {
		waitUntil: "domcontentloaded",
	});
	await page.waitForFunction(() => window.__bench?.ready === true, null, {
		timeout: 120_000,
	});
	if (phase === "stream") {
		await page.waitForFunction(() => window.__bench?.streamDone === true, null, {
			timeout: 120_000,
		});
	} else {
		await page.waitForTimeout(500);
	}
	const summary = await page.evaluate(() => window.__bench.summary());

	await cdp.send("Tracing.end");
	await done;

	// Buffered mode (no transferMode) delivers every event through
	// dataCollected, which is what the aggregation below reads.
	const byName = new Map();
	for (const event of events) {
		if (event.ph !== "X" && event.ph !== "B") continue;
		const dur =
			event.ph === "X"
				? (event.dur ?? 0)
				: 0; // B events need pairing; X events carry duration.
		if (dur <= 0) continue;
		byName.set(event.name, (byName.get(event.name) ?? 0) + dur);
	}

	const byCategory = new Map();
	for (const [name, us] of byName) {
		const hint = CATEGORY_HINTS.find(([n]) => n === name);
		if (!hint) continue;
		byCategory.set(hint[1], (byCategory.get(hint[1]) ?? 0) + us);
	}

	console.log(`scenario: ${scenario} (phase: ${phase})`);
	console.log("bench summary:", JSON.stringify(summary));
	console.log("\ntime by category (main thread):");
	for (const [category, us] of [...byCategory.entries()].sort(
		(a, b) => b[1] - a[1],
	)) {
		console.log(`  ${String(Math.round(us / 1000)).padStart(7)}ms  ${category}`);
	}
	console.log("\ntop trace events by total duration:");
	for (const [name, us] of [...byName.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, 25)) {
		console.log(`  ${String(Math.round(us / 1000)).padStart(7)}ms  ${name}`);
	}

	// FunctionCall carries the JS function name and script URL in args.data,
	// which is what names the actual hot function instead of a bundle chunk.
	// Durations here are inclusive (nested calls count inside their caller),
	// so rank by the shallowest frames only.
	const fnTotal = new Map();
	const depths = new Map();
	for (const event of events) {
		if (event.ph !== "X" || event.name !== "FunctionCall") continue;
		const dur = event.dur ?? 0;
		if (dur <= 0) continue;
		const data = event.args?.data ?? {};
		const fn = data.functionName || "(anonymous)";
		const url = (data.url ?? "").split("/").pop() ?? "";
		const line = data.lineNumber ?? -1;
		const key = `${fn} [${url}:${line}]`;
		const prev = fnTotal.get(key) ?? { ms: 0, count: 0, depth: Number.MAX_SAFE_INTEGER };
		prev.ms += dur;
		prev.count += 1;
		prev.depth = Math.min(prev.depth, (event.args?.data?.stack?.length ?? 0) + 1);
		fnTotal.set(key, prev);
	}
	// Exclusive estimate: subtract each frame's children of the same name
	// is unreliable in a flat trace, so report the top-level frames instead,
	// which bound the work attributed to a function.
	const topLevel = [...fnTotal.entries()]
		.filter(([, v]) => v.depth <= 1)
		.sort((a, b) => b[1].ms - a[1].ms)
		.slice(0, 20);
	console.log("\ntop-level JS function frames (inclusive, depth<=1):");
	for (const [key, value] of topLevel) {
		console.log(
			`  ${String(Math.round(value.ms / 1000)).padStart(7)}ms  ${String(value.count).padStart(6)}x  ${key}`,
		);
	}

	const jsFns = [...fnTotal.entries()]
		.sort((a, b) => b[1].ms - a[1].ms)
		.slice(0, 20);
	console.log("\ntop JS functions (inclusive, any depth):");
	for (const [key, value] of jsFns) {
		console.log(
			`  ${String(Math.round(value.ms / 1000)).padStart(7)}ms  ${String(value.count).padStart(6)}x  ${key}`,
		);
	}

	fs.writeFileSync(outFile, JSON.stringify({ summary, events: events.length }));
	await browser.close();
	server?.kill("SIGTERM");
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
