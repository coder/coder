// Per-row cost model, measured rather than inferred.
//
// Earlier numbers mixed a rich message shape with several panels. This sweeps
// total mounted rows at one panel and at six panels, for both a minimal row
// (tiny) and a realistic row (rich), in both engines. The slope is the
// per-mounted-row main-thread cost; the intercept is the fixed shell cost.
// Those two numbers decide the window size that meets a block budget.
import { chromium, webkit } from "@playwright/test";

const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const points = [];

const sample = async (browserType, engine, panels, windowRows, content) => {
	const browser = await browserType.launch();
	const context = await browser.newContext({
		viewport: { width: 1512, height: 950 },
	});
	const page = await context.newPage();
	const params = new URLSearchParams({
		panels: String(panels),
		// Transcript is long enough that the window is always the binding
		// constraint, so mounted rows == panels * windowRows.
		turns: "200",
		streamChars: "0",
		content,
		windowRows: String(windowRows),
	});
	await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${params}`, {
		waitUntil: "domcontentloaded",
	});
	await page.waitForFunction(() => window.__bench?.ready === true, null, {
		timeout: 180_000,
	});
	const phase = await page.evaluate(() => window.__bench.phaseSummary("mount"));
	const rows = await page.evaluate(
		() => document.querySelectorAll('[data-testid^="chat-message-"]').length,
	);
	await context.close();
	await browser.close();
	points.push({
		engine,
		panels,
		content,
		rows,
		block: phase.worstBlockMs,
	});
};

const report = (engine, content) => {
	const subset = points.filter(
		(p) => p.engine === engine && p.content === content,
	);
	if (subset.length < 2) return;
	// Least squares over mounted rows.
	const n = subset.length;
	const sx = subset.reduce((s, p) => s + p.rows, 0);
	const sy = subset.reduce((s, p) => s + p.block, 0);
	const sxx = subset.reduce((s, p) => s + p.rows * p.rows, 0);
	const sxy = subset.reduce((s, p) => s + p.rows * p.block, 0);
	const slope = (n * sxy - sx * sy) / (n * sxx - sx * sx);
	const intercept = (sy - slope * sx) / n;
	console.log(
		`${engine} ${content}: block ≈ ${intercept.toFixed(0)}ms + ${slope.toFixed(2)}ms x mountedRows`,
	);
	for (const p of subset) {
		console.log(
			`    panels=${p.panels} windowRows=${p.rows / p.panels} rows=${String(p.rows).padStart(4)} -> ${String(p.block).padStart(7)}ms`,
		);
	}
};

const main = async () => {
	for (const content of ["tiny", "rich"]) {
		for (const [engine, browserType] of [
			["Chromium", chromium],
			["WebKit", webkit],
		]) {
			for (const panels of [1, 6]) {
				for (const windowRows of [5, 10, 20, 40]) {
					await sample(browserType, engine, panels, windowRows, content);
				}
			}
			report(engine, content);
		}
	}
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});