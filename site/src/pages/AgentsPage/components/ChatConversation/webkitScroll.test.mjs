/**
 * WebKit scroll verification for the sticky message refactor.
 *
 * Uses Playwright's WebKit engine to verify that:
 * 1. Scroll position does not drift on an idle chat.
 * 2. Scroll position stays pinned to bottom during content
 *    growth.
 * 3. CSS sticky push-up works with section-wrapped turns.
 *
 * Run: cd site && node src/pages/AgentsPage/components/ChatConversation/webkitScroll.test.mjs
 *
 * @module
 */

/* biome-ignore-all lint/suspicious/noConsole: test script */

import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { webkit } = require("@playwright/test");

/**
 * Builds an HTML fixture replicating the scroll container and
 * section-wrapped sticky message structure.
 */
function buildFixture(turnCount) {
	let turns = "";
	for (let i = 0; i < turnCount; i++) {
		turns += `
			<div class="turn" style="display:flex;flex-direction:column;gap:12px">
				<div data-user-sentinel style="height:0"></div>
				<div class="sticky-msg" style="position:sticky;top:0;z-index:10;padding:8px;background:#f0f0f0">
					User message ${i + 1}
				</div>
				<div style="min-height:400px;padding:8px;background:#fafafa">
					Assistant response ${i + 1}
				</div>
			</div>`;
	}
	return `
		<div id="scroller" style="height:500px;overflow-y:auto;overflow-anchor:none;overscroll-behavior:contain">
			<div id="content" style="display:flex;flex-direction:column;gap:12px">
				${turns}
			</div>
		</div>`;
}

let failed = 0;

function assert(condition, msg) {
	if (!condition) {
		console.error(`  FAIL: ${msg}`);
		failed++;
	} else {
		console.log(`  PASS: ${msg}`);
	}
}

async function main() {
	const browser = await webkit.launch();
	const page = await browser.newPage();

	console.log("Test 1: idle scroll stability");
	await page.setContent(buildFixture(5));
	await page.evaluate(() => {
		const s = document.getElementById("scroller");
		s.scrollTop = s.scrollHeight - s.clientHeight;
	});
	const initial = await page.evaluate(
		() => document.getElementById("scroller").scrollTop,
	);
	await page.evaluate(
		() =>
			new Promise((resolve) => {
				let count = 0;
				function tick() {
					if (++count >= 30) resolve();
					else requestAnimationFrame(tick);
				}
				requestAnimationFrame(tick);
			}),
	);
	const after = await page.evaluate(
		() => document.getElementById("scroller").scrollTop,
	);
	assert(
		Math.abs(after - initial) <= 1,
		`scrollTop stable: ${initial} -> ${after}`,
	);

	console.log("Test 2: content growth stability");
	await page.setContent(buildFixture(3));
	await page.evaluate(() => {
		const s = document.getElementById("scroller");
		s.scrollTop = s.scrollHeight - s.clientHeight;
	});
	const beforeGrowth = await page.evaluate(
		() => document.getElementById("scroller").scrollTop,
	);
	await page.evaluate(() => {
		const content = document.getElementById("content");
		const extra = document.createElement("div");
		extra.style.height = "200px";
		extra.textContent = "New streamed content";
		content.appendChild(extra);
	});
	await page.evaluate(
		() =>
			new Promise((resolve) =>
				requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
			),
	);
	const afterGrowth = await page.evaluate(
		() => document.getElementById("scroller").scrollTop,
	);
	assert(
		afterGrowth >= beforeGrowth,
		`no backward jump: ${beforeGrowth} -> ${afterGrowth}`,
	);
	assert(afterGrowth > 0, `scrollTop not zero: ${afterGrowth}`);

	console.log("Test 3: sticky push-up");
	await page.setContent(buildFixture(3));
	await page.evaluate(() => {
		document.getElementById("scroller").scrollTop = 500;
	});
	await page.evaluate(
		() => new Promise((resolve) => requestAnimationFrame(() => resolve())),
	);
	const positions = await page.evaluate(() => {
		const scroller = document.getElementById("scroller");
		const scrollerRect = scroller.getBoundingClientRect();
		return Array.from(document.querySelectorAll(".sticky-msg")).map((h) => {
			const rect = h.getBoundingClientRect();
			return {
				top: rect.top - scrollerRect.top,
				visible:
					rect.bottom > scrollerRect.top && rect.top < scrollerRect.bottom,
			};
		});
	});
	assert(positions[1].visible, "second sticky header is visible");
	assert(positions[1].top < 50, `second header near top: ${positions[1].top}`);

	await browser.close();
	console.log(`\n${failed === 0 ? "All passed" : `${failed} failed`}`);
	process.exit(failed > 0 ? 1 : 0);
}

main().catch((e) => {
	console.error(e);
	process.exit(1);
});
