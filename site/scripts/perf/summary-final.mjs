// Final before/after summary against the block budget.
//
// Compares the shipped default with windowing disabled, at several panel
// counts, in both engines, with repeated trials and medians. "Off" uses the
// bench-only perfCv=off hook, which appends a style rule that cancels the row
// skip, so both arms come from one bundle.
import { chromium, webkit } from "@playwright/test";

const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";
const TRIALS = Number(process.env.TRIALS ?? 3);

const median = (values) => {
	const sorted = [...values].sort((a, b) => a - b);
	return sorted[Math.floor(sorted.length / 2)];
};

const measure = async (browserType, panels, extra) => {
	const browser = await browserType.launch();
	const blocks = [];
	const walls = [];
	const rowsSeen = [];
	for (let i = 0; i < TRIALS; i++) {
		const context = await browser.newContext({
			viewport: { width: 1512, height: 950 },
		});
		const page = await context.newPage();
		const params = new URLSearchParams({
			panels: String(panels),
			turns: "100",
			streamChars: "0",
			...extra,
		});
		await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${params}`, {
			waitUntil: "domcontentloaded",
		});
		await page.waitForFunction(() => window.__bench?.ready === true, null, {
			timeout: 180_000,
		});
		const phase = await page.evaluate(() =>
			window.__bench.phaseSummary("mount"),
		);
		blocks.push(phase.worstBlockMs);
		walls.push(await page.evaluate(() => Math.round(performance.now())));
		rowsSeen.push(
			await page.evaluate(
				() =>
					document.querySelectorAll('[data-testid^="chat-message-"]').length,
			),
		);
		await context.close();
	}
	await browser.close();
	return {
		block: median(blocks),
		wall: median(walls),
		rows: median(rowsSeen),
	};
};

const main = async () => {
	console.log(
		`medians of ${TRIALS} trials, 100-turn chats, but the transcript is long enough that the window binds\n`,
	);
	for (const [engine, browserType] of [
		["Chromium", chromium],
		["WebKit  ", webkit],
	]) {
		for (const panels of [1, 2, 4, 6]) {
			const off = await measure(browserType, panels, { perfCv: "off" });
			const on = await measure(browserType, panels, {});
			console.log(
				`${engine} ${panels} chat${panels > 1 ? "s" : ""}:  ` +
					`before block=${String(off.block).padStart(7)}ms (rows ${String(off.rows).padStart(4)})   ` +
					`after block=${String(on.block).padStart(7)}ms (rows ${String(on.rows).padStart(3)})   ` +
					`wall ${String(off.wall).padStart(5)}ms -> ${String(on.wall).padStart(5)}ms`,
			);
		}
	}
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});