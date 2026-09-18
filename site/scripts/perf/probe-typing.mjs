// Keystroke latency while chats stream.
//
// Reported symptom: with two chats open and streaming, focusing the composer
// is slow and typing "when" can take ~3s to appear. This measures the two
// parts of that: how long the main thread is busy (which delays any input
// handling) and how long a real key press takes to be reflected in the DOM.
//
// The bench page renders the transcript panels; this probe adds a plain
// contenteditable input and types into it with real key events, so the
// measurement includes whatever else is competing for the main thread.
import { chromium, webkit } from "@playwright/test";

const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const run = async (browserType, label, options) => {
	const { panels, streamChars, windowRows } = options;
	const browser = await browserType.launch();
	const context = await browser.newContext({
		viewport: { width: 1512, height: 950 },
	});
	const page = await context.newPage();
	const params = new URLSearchParams({
		panels: String(panels),
		turns: "100",
		streamChars: String(streamChars),
		content: "rich",
		...(windowRows ? { windowRows: String(windowRows) } : {}),
	});
	await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${params}`, {
		waitUntil: "domcontentloaded",
	});
	await page.waitForFunction(() => window.__bench?.ready === true, null, {
		timeout: 180_000,
	});

	// A real focused text input: typing into it goes through the same main
	// thread that the streaming transcript is competing for.
	await page.evaluate(() => {
		const input = document.createElement("input");
		input.id = "latency-probe";
		input.style.cssText =
			"position:fixed;top:0;left:0;width:200px;height:30px;z-index:99999;";
		document.body.appendChild(input);
	});
	await page.focus("#latency-probe");

	// Record focus responsiveness: how long the focus request took to land.
	const focusStart = Date.now();
	await page.waitForFunction(
		() => document.activeElement?.id === "latency-probe",
		null,
		{ timeout: 30_000 },
	);
	const focusMs = Date.now() - focusStart;

	// Type a short word and measure how long each character takes to appear in
	// the input's value, which is what "typing takes 3s to show up" describes.
	const perKey = [];
	const word = "when";
	for (let i = 0; i < word.length; i++) {
		const t0 = Date.now();
		await page.keyboard.type(word[i]);
		await page.waitForFunction(
			(expected) => document.querySelector("#latency-probe")?.value === expected,
			word.slice(0, i + 1),
			{ timeout: 30_000 },
		);
		perKey.push(Date.now() - t0);
	}
	const sorted = [...perKey].sort((a, b) => a - b);
	const streamPhase = await page.evaluate(() =>
		window.__bench.phaseSummary("stream"),
	);
	// Starvation ratio: the fraction of the streaming window during which the
	// main thread had no capacity to handle input. Saturated main thread is
	// what makes keystrokes wait for seconds even when each block is modest.
	const starvation =
		streamPhase.durationMs > 0
			? Math.round((streamPhase.blockedMs / streamPhase.durationMs) * 100)
			: 0;

	console.log(`${label}:`);
	console.log(
		`  focus=${focusMs}ms  key-to-DOM p50=${sorted[Math.floor(sorted.length / 2)]}ms  worst=${sorted.at(-1)}ms  (per key: ${perKey.join(", ")}ms)`,
	);
	console.log(
		`  stream: worst block=${streamPhase.worstBlockMs}ms  blocked=${streamPhase.blockedMs}ms of ${streamPhase.durationMs}ms (${starvation}% starved)`,
	);
	await context.close();
	await browser.close();
};

const main = async () => {
	for (const [engine, browserType] of [
		["Chromium", chromium],
		["WebKit  ", webkit],
	]) {
		// Idle baseline: no streaming, so nothing competes for the thread.
		await run(browserType, `${engine} 2 chats idle`, {
			panels: 2,
			streamChars: 0,
		});
		// Streaming with every row mounted: the reported configuration.
		await run(browserType, `${engine} 2 chats streaming, all rows`, {
			panels: 2,
			streamChars: 6000,
			windowRows: 100000,
		});
		// Same, with the row window fix applied.
		await run(browserType, `${engine} 2 chats streaming, windowed`, {
			panels: 2,
			streamChars: 6000,
		});
	}
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});