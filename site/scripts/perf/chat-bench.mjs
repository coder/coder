// Real-browser benchmark driver for chat rendering.
//
// Serves the production build (with the standalone bench entry) from a
// self-contained static server and drives it in headless Chromium. The page
// renders the real transcript components against a real ChatStore, so the
// measured main-thread cost is the app's own.
//
// Usage:
//   node scripts/perf/chat-bench.mjs [--trials 3] [--json out.json] [scenario...]
import { chromium, webkit } from "@playwright/test";
import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

// BENCH_BROWSER selects the engine. webkit is Safari's engine, which is the
// configuration the original report calls out as almost unusable, so both are
// measured rather than assuming they behave alike.
const BROWSERS = { chromium, webkit };

const siteRoot = path.resolve(import.meta.dirname, "../..");
const outDir = path.join(siteRoot, "out");
const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const scenarios = [
	// Chat switching: mount a transcript and let it settle, no streaming.
	{ name: "switch-1x25", params: { turns: 25, panels: 1, streamChars: 0 } },
	{ name: "switch-1x100", params: { turns: 100, panels: 1, streamChars: 0 } },
	// Multiple chats at once: the case the report says makes Safari unusable.
	{ name: "multi-4x25", params: { turns: 25, panels: 4, streamChars: 0 } },
	{ name: "multi-4x100", params: { turns: 100, panels: 4, streamChars: 0 } },
	// Streaming: the hot path, short and long transcripts.
	{ name: "stream-1x25", params: { turns: 25, panels: 1, streamChars: 6000 } },
	{ name: "stream-1x100", params: { turns: 100, panels: 1, streamChars: 6000 } },
	{
		name: "stream-4x100",
		params: { turns: 100, panels: 4, streamChars: 6000 },
	},
	// Publishing cadence at constant total text. Same 6000 chars streamed,
	// once in 250 small ticks and once in 50 large ticks. If cost tracks
	// tick count, the cost is per-publish overhead; if it tracks total text,
	// the cost is the size of each render.
	{
		name: "cadence-250ticks",
		params: { turns: 100, panels: 1, streamChars: 6000, charsPerTick: 24 },
	},
	{
		name: "cadence-50ticks",
		params: { turns: 100, panels: 1, streamChars: 6000, charsPerTick: 120 },
	},
	// Content-shape controls, at a fixed message count. These isolate which
	// rendering sub-system owns the cost: markdown prose only, fenced code
	// only (highlighting), or the full rich mix.
	{
		name: "shape-prose-100",
		params: { turns: 100, panels: 1, streamChars: 0, content: "prose" },
	},
	{
		name: "shape-code-100",
		params: { turns: 100, panels: 1, streamChars: 0, content: "code" },
	},
	{
		name: "shape-rich-100",
		params: { turns: 100, panels: 1, streamChars: 0, content: "rich" },
	},
	// The reported case: several long chats visible at once, each with a tall
	// transcript, so total mounted rows are what the user waits on.
	{
		name: "multi-6x100",
		params: { turns: 100, panels: 6, streamChars: 0 },
	},
	// Windowing series: mounted row count per chat is the variable that sets
	// the main-thread block, so these bracket the size that meets a 250ms
	// target for several chats at once.
	{
		name: "win-6x100-60",
		params: { turns: 100, panels: 6, streamChars: 0, windowRows: 60 },
	},
	{
		name: "win-6x100-30",
		params: { turns: 100, panels: 6, streamChars: 0, windowRows: 30 },
	},
	{
		name: "win-6x100-15",
		params: { turns: 100, panels: 6, streamChars: 0, windowRows: 15 },
	},
	{
		name: "win-1x100-60",
		params: { turns: 100, panels: 1, streamChars: 0, windowRows: 60 },
	},
	{
		name: "win-1x400-60",
		params: { turns: 400, panels: 1, streamChars: 0, windowRows: 60 },
	},
	{
		name: "win-1x2000-60",
		params: { turns: 2000, panels: 1, streamChars: 0, windowRows: 60 },
	},
	// Mount scaling series: isolates fixed page/startup cost (turns=0) from
	// per-message transcript cost, and the per-message cost of syntax
	// highlighting (prose vs code at the same message count).
	{
		name: "empty-0",
		params: { turns: 0, panels: 1, streamChars: 0, content: "prose" },
	},
	{
		name: "prose-25",
		params: { turns: 25, panels: 1, streamChars: 0, content: "prose" },
	},
	{
		name: "prose-50",
		params: { turns: 50, panels: 1, streamChars: 0, content: "prose" },
	},
	{
		name: "code-25",
		params: { turns: 25, panels: 1, streamChars: 0, content: "code" },
	},
	{
		name: "code-50",
		params: { turns: 50, panels: 1, streamChars: 0, content: "code" },
	},
];

const readArgs = (argv) => {
	const parsed = { trials: 3, json: null, names: [], extra: {} };
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (arg === "--trials") {
			parsed.trials = Number(argv[++i]);
		} else if (arg === "--json") {
			parsed.json = argv[++i];
		} else if (arg === "--extra") {
			// --extra key=value[,key=value]
			for (const pair of String(argv[++i]).split(",")) {
				const [key, value] = pair.split("=");
				parsed.extra[key] = value;
			}
		} else if (arg.startsWith("--")) {
			throw new Error(`unknown flag ${arg}`);
		} else {
			parsed.names.push(arg);
		}
	}
	return parsed;
};

const flags = readArgs(process.argv.slice(2));
const TRIALS = flags.trials;
const JSON_OUT = flags.json;
const active = flags.names.length
	? scenarios.filter((s) => flags.names.includes(s.name))
	: scenarios;
// Extra query params appended to every scenario URL. Used to A/B a fix
// inside a single build, which removes build variance from the comparison.
const extraParams = flags.extra ?? {};

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
		child.stderr.on("data", (chunk) => {
			const text = chunk.toString();
			if (!text.includes("EADDRINUSE")) process.stderr.write(text);
		});
		child.on("error", reject);
		setTimeout(() => reject(new Error("bench server did not start")), 10_000);
	});

/**
 * Reuses a server already listening on the bench port, otherwise spawns one.
 * Reuse keeps repeated baseline/treatment runs on the same server process
 * and does not fail when an earlier run left one behind.
 */
const ensureServer = async () => {
	if (await serverReady()) {
		return null;
	}
	return startServer();
};

const runScenario = async (browser, scenario) => {
	const results = [];
	for (let trial = 0; trial < TRIALS; trial++) {
		const context = await browser.newContext({
			viewport: { width: 1512, height: 950 },
			deviceScaleFactor: 1,
		});
		const page = await context.newPage();
		const url = `http://127.0.0.1:${PORT}${BENCH_PATH}?${new URLSearchParams({
			...Object.fromEntries(
				Object.entries(scenario.params).map(([k, v]) => [k, String(v)]),
			),
			...extraParams,
		})}`;
		await page.goto(url, { waitUntil: "domcontentloaded" });
		await page.waitForFunction(() => window.__bench?.ready === true, null, {
			timeout: 120_000,
		});
		// Mount phase: transcript mount and settle, the cost a chat switch
		// pays. Sampled before the stream starts.
		const mount = await page.evaluate(() =>
			window.__bench.phaseSummary("mount"),
		);
		// Wall-clock time from navigation start to the harness reporting ready.
		// longtask is a Chromium-only PerformanceObserver type, so WebKit runs
		// need this and the frame-gap stats to be comparable.
		const mountWallMs = await page.evaluate(() =>
			Math.round(performance.now()),
		);
		const mountCounters = await page.evaluate(() => window.__bench.counters());
		const streamParams = scenario.params.streamChars ?? 0;
		if (streamParams > 0) {
			await page.waitForFunction(
				() => window.__bench?.streamDone === true,
				null,
				{ timeout: 120_000 },
			);
		} else {
			// No stream: sample a fixed settle window instead.
			await page.waitForTimeout(1500);
		}
		const summary = await page.evaluate(() => window.__bench.summary());
		const streamPhase = await page.evaluate(() =>
			window.__bench.phaseSummary("stream"),
		);
		const domNodes = await page.evaluate(
			() => document.querySelectorAll("*").length,
		);
		const rows = await page.evaluate(
			() =>
				document.querySelectorAll('[data-testid^="chat-message-"]').length,
		);
		const counters = await page.evaluate(() => window.__bench.counters());
		// Streaming delta: counters minus what mount already consumed. This
		// is the per-tick workload the streaming phase actually pays.
		const streamCounters = {};
		for (const [key, value] of Object.entries(counters)) {
			if (typeof value === "number") {
				streamCounters[key] = value - (mountCounters[key] ?? 0);
			}
		}
		await context.close();
		results.push({
			mount,
			mountWallMs,
			streamPhase,
			total: summary,
			domNodes,
			rows,
			counters: streamCounters,
		});
	}
	return results;
};

const median = (values) => {
	const sorted = [...values].sort((a, b) => a - b);
	return sorted[Math.floor(sorted.length / 2)];
};

const main = async () => {
	const server = await ensureServer();
	const engine = process.env.BENCH_BROWSER ?? "chromium";
	const browserType = BROWSERS[engine];
	if (!browserType) {
		throw new Error(`unknown BENCH_BROWSER ${engine}`);
	}
	console.log(`browser: ${engine}`);
	const browser = await browserType.launch();
	const report = {};
	try {
		for (const scenario of active) {
			const trials = await runScenario(browser, scenario);
			const agg = {
				// Mount (chat switch) cost.
				mountLongTaskMs: median(trials.map((t) => t.mount.longTaskMs)),
				mountWorstTaskMs: median(trials.map((t) => t.mount.worstLongTaskMs)),
				// Engine-agnostic mount metrics: these work in WebKit too.
				mountWallMs: median(trials.map((t) => t.mountWallMs)),
				mountFrames: median(trials.map((t) => t.mount.frames)),
				mountWorstGapMs: median(trials.map((t) => t.mount.worstFrameGapMs)),
				mountWorstBlockMs: median(trials.map((t) => t.mount.worstBlockMs)),
				mountBlockedMs: median(trials.map((t) => t.mount.blockedMs)),
				mountFramesOver50ms: median(
					trials.map((t) => t.mount.framesOver50ms),
				),
				// Streaming cost, isolated from mount.
				streamLongTaskMs: median(
					trials.map((t) => t.streamPhase.longTaskMs),
				),
				streamWorstTaskMs: median(
					trials.map((t) => t.streamPhase.worstLongTaskMs),
				),
				streamFramesOver50ms: median(
					trials.map((t) => t.streamPhase.framesOver50ms),
				),
				streamWorstGapMs: median(
					trials.map((t) => t.streamPhase.worstFrameGapMs),
				),
				streamWorstBlockMs: median(
					trials.map((t) => t.streamPhase.worstBlockMs),
				),
				streamBlockedMs: median(
					trials.map((t) => t.streamPhase.blockedMs),
				),
				streamDurationMs: median(
					trials.map((t) => t.streamPhase.durationMs),
				),
				totalLongTaskMs: median(trials.map((t) => t.total.longTaskMs)),
				domNodes: median(trials.map((t) => t.domNodes)),
				rows: median(trials.map((t) => t.rows)),
				parseCalls: median(trials.map((t) => t.counters.parseCalls ?? 0)),
				messagesParsed: median(trials.map((t) => t.counters.messagesParsed ?? 0)),
				timelineMounts: median(trials.map((t) => t.counters.timelineMounts ?? 0)),
				parseLens: trials[0].counters.parseLens ?? [],
				displayMessageRuns: median(
					trials.map((t) => t.counters.displayMessageRuns ?? 0),
				),
				displayMessageInputs: median(
					trials.map((t) => t.counters.displayMessageInputs ?? 0),
				),
				itemRenders: median(trials.map((t) => t.counters.itemRenders ?? 0)),
				liveRowRenders: median(
					trials.map((t) => t.counters.liveRowRenders ?? 0),
				),
				historicalRowRenders: median(
					trials.map((t) => t.counters.historicalRowRenders ?? 0),
				),
				markdownRuns: median(trials.map((t) => t.counters.markdownRuns ?? 0)),
				markdownChars: median(
					trials.map((t) => t.counters.markdownChars ?? 0),
				),
				timelineRenders: median(
					trials.map((t) => t.counters.timelineRenders ?? 0),
				),
				trials: trials.length,
			};
			report[scenario.name] = agg;
			console.log(
				`${scenario.name.padEnd(14)} ` +
					`mountWall=${String(agg.mountWallMs).padStart(6)}ms  ` +
					`mountBlock=${String(agg.mountWorstBlockMs).padStart(7)}ms  ` +
					`mountBlocked=${String(agg.mountBlockedMs).padStart(6)}ms  ` +
					`streamWall=${String(agg.streamDurationMs).padStart(6)}ms  ` +
					`streamBlock=${String(agg.streamWorstBlockMs).padStart(7)}ms  ` +
					`streamBlocked=${String(agg.streamBlockedMs).padStart(6)}ms  ` +
					`nodes=${String(agg.domNodes).padStart(5)}`,
			);
			// Repeated identical input sizes mean the same work is being redone.
		const lens = report[scenario.name].parseLens;
		if (lens.length > 1) {
			const distinct = new Set(lens);
			console.log(
				`  parse inputs: ${lens.length} calls, ${distinct.size} distinct sizes ` +
					`(${[...distinct].slice(0, 8).join(", ")})`,
			);
		}
		}
	} finally {
		await browser.close();
		server?.kill("SIGTERM");
	}
	if (JSON_OUT) {
		fs.writeFileSync(JSON_OUT, JSON.stringify(report, null, 2));
	}
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
