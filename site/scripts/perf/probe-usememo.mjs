// Is `useMemo` in ChatPageTimeline redundant under React Compiler?
//
// The compiler guards many expressions automatically, so a useMemo can be
// redundant. It can also stop being effective when a component exceeds the
// compiler's practical complexity ceiling, or when the guarded expression sits
// behind an unstable dependency. This measures the runtime answer directly:
// it counts how many times the transcript parse actually runs while a stream
// ticks, with the memos in place and with them removed.
//
// Runs against an already-built bench bundle, so the memo variant must be
// built by the caller. It reads the counter the app exposes via its opt-in
// instrumentation, which is only active in the bench harness.
import { chromium } from "@playwright/test";

const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const measure = async (browser, label) => {
	const context = await browser.newContext({
		viewport: { width: 1512, height: 950 },
	});
	const page = await context.newPage();
	const params = new URLSearchParams({
		panels: "1",
		turns: "100",
		streamChars: "6000",
		content: "rich",
	});
	await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${params}`, {
		waitUntil: "domcontentloaded",
	});
	await page.waitForFunction(() => window.__bench?.ready === true, null, {
		timeout: 180_000,
	});
	const mountCounters = await page.evaluate(() => window.__bench.counters());
	await page.waitForFunction(() => window.__bench?.streamDone === true, null, {
		timeout: 180_000,
	});
	const finalCounters = await page.evaluate(() => window.__bench.counters());
	const phase = await page.evaluate(() => window.__bench.phaseSummary("stream"));

	const streamParses =
		(finalCounters.parseCalls ?? 0) - (mountCounters.parseCalls ?? 0);
	const streamMsgs =
		(finalCounters.messagesParsed ?? 0) - (mountCounters.messagesParsed ?? 0);
	console.log(`${label}:`);
	console.log(
		`  transcript parses during stream: ${streamParses} (messages walked: ${streamMsgs})`,
	);
	console.log(
		`  stream phase: worst block=${phase.worstBlockMs}ms blocked=${phase.blockedMs}ms of ${phase.durationMs}ms`,
	);
	await context.close();
};

const main = async () => {
	const browser = await chromium.launch();
	await measure(browser, process.env.LABEL ?? "build under test");
	await browser.close();
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
