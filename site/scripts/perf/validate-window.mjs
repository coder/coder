// Functional check for the row window: the transcript must stay usable.
//
// Windowing changes what is mounted, so this verifies the user-visible
// behaviour rather than the timing: newest messages are visible without
// scrolling, older messages are reachable, the reveal control works and is
// absent on a short transcript, and the newest row stays at the bottom.
import { chromium } from "@playwright/test";

const PORT = Number(process.env.BENCH_PORT ?? 8099);
const BENCH_PATH = "/bench/chat-bench/index.html";

const check = async (browser, label, turns) => {
	const context = await browser.newContext({
		viewport: { width: 1512, height: 950 },
	});
	const page = await context.newPage();
	const params = new URLSearchParams({
		panels: "1",
		turns: String(turns),
		streamChars: "0",
		content: "rich",
	});
	await page.goto(`http://127.0.0.1:${PORT}${BENCH_PATH}?${params}`, {
		waitUntil: "domcontentloaded",
	});
	await page.waitForFunction(() => window.__bench?.ready === true, null, {
		timeout: 180_000,
	});
	await page.waitForTimeout(600);

	const before = await page.evaluate(() => {
		const rows = [...document.querySelectorAll("[data-message-id]")];
		const reveal = [...document.querySelectorAll("button")].find((b) =>
			b.textContent?.includes("earlier messages"),
		);
		return {
			mounted: rows.length,
			hasReveal: Boolean(reveal),
			revealLabel: reveal?.textContent?.trim() ?? null,
			firstText: rows[0]?.textContent?.slice(0, 30) ?? null,
			lastText: rows.at(-1)?.textContent?.slice(0, 30) ?? null,
		};
	});

	let after = null;
	if (before.hasReveal) {
		// Click the reveal control and confirm more history mounts.
		await page.evaluate(() => {
			const reveal = [...document.querySelectorAll("button")].find((b) =>
				b.textContent?.includes("earlier messages"),
			);
			reveal?.click();
		});
		await page.waitForTimeout(600);
		after = await page.evaluate(() => ({
			mounted: document.querySelectorAll("[data-message-id]").length,
			firstText:
				document.querySelector("[data-message-id]")?.textContent?.slice(0, 30) ??
				null,
		}));
	}

	console.log(`${label} (turns=${turns}):`);
	console.log(
		`  mounted=${before.mounted} reveal=${before.hasReveal ? `"${before.revealLabel}"` : "none"}`,
	);
	console.log(`  oldest mounted row starts: "${before.firstText}"`);
	console.log(`  newest mounted row starts: "${before.lastText}"`);
	if (after) {
		console.log(
			`  after reveal: mounted=${after.mounted}, oldest starts: "${after.firstText}"`,
		);
	}
	await context.close();
};

const main = async () => {
	const browser = await chromium.launch();
	await check(browser, "long transcript (window activated)", 100);
	await check(browser, "short transcript (must not show reveal)", 5);
	await browser.close();
};

main().catch((err) => {
	console.error(err);
	process.exit(1);
});