// What does a single "tiny" row actually contain, and where does its
// per-row cost come from?
//
// A one-line message costs ~2.2ms of main-thread block in WebKit. That is far
// more than laying out one line of text, so the cost must be per-row
// structure: component count, DOM nodes and providers, not characters. This
// reports the node/component shape of a single row so the per-row overhead can
// be attributed instead of guessed at.
import { chromium, webkit } from "@playwright/test";

const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const inspect = async (browserType, label) => {
	const browser = await browserType.launch();
	const context = await browser.newContext({
		viewport: { width: 1512, height: 950 },
	});
	const page = await context.newPage();
	const params = new URLSearchParams({
		panels: "1",
		turns: "2",
		streamChars: "0",
		content: "tiny",
	});
	await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${params}`, {
		waitUntil: "domcontentloaded",
	});
	await page.waitForFunction(() => window.__bench?.ready === true, null, {
		timeout: 180_000,
	});
	const info = await page.evaluate(() => {
		const rows = [...document.querySelectorAll("[data-message-id]")];
		const describe = (row) => {
			const all = row.querySelectorAll("*");
			const byTag = {};
			for (const el of all) {
				const tag = el.tagName.toLowerCase();
				byTag[tag] = (byTag[tag] ?? 0) + 1;
			}
			return {
				nodes: all.length,
				byTag,
				text: (row.textContent ?? "").slice(0, 40),
			};
		};
		return {
			rowCount: rows.length,
			rows: rows.map(describe),
		};
	});
	console.log(`\n${label}`);
	for (const row of info.rows) {
		const top = Object.entries(row.byTag)
			.sort((a, b) => b[1] - a[1])
			.slice(0, 10)
			.map(([tag, n]) => `${tag}:${n}`)
			.join(" ");
		console.log(`  nodes=${String(row.nodes).padStart(3)}  text="${row.text}"`);
		console.log(`    ${top}`);
	}
	await context.close();
	await browser.close();
};

const main = async () => {
	await inspect(webkit, "WebKit: one tiny user row + one tiny assistant row");
	await inspect(chromium, "Chromium: same");
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});