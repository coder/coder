// Screenshot a story from a running Storybook dev server.
//
// Usage: pnpm storybook:screenshot <story-id> [-o out.png] [-t light]
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { chromium } from "@playwright/test";
import {
	PLAY_FUNCTION_THREW_EXCEPTION,
	STORY_ERRORED,
	STORY_RENDERED,
	STORY_THREW_EXCEPTION,
} from "storybook/internal/core-events";

const usage = "Usage: pnpm storybook:screenshot <story-id> [-o out.png] [-t light]";
const base = process.env.STORYBOOK_URL ?? "http://localhost:6006/";

/**
 * Loads a story and resolves with the Storybook event that ended its render:
 * STORY_RENDERED on success (after play functions), or a failure event.
 */
const loadStory = async (page, url) => {
	await page.addInitScript((events) => {
		// The preview is created by a module script, which runs before
		// DOMContentLoaded; the first render starts after.
		document.addEventListener("DOMContentLoaded", () => {
			const { channel } = globalThis.__STORYBOOK_PREVIEW__;
			for (const event of events) {
				channel.on(event, () => {
					globalThis.__storyRenderEvent ??= event;
				});
			}
		});
	}, [
		STORY_RENDERED,
		STORY_ERRORED,
		STORY_THREW_EXCEPTION,
		PLAY_FUNCTION_THREW_EXCEPTION,
	]);
	await page.goto(url.href);
	const event = await page.waitForFunction(() => globalThis.__storyRenderEvent);
	return event.jsonValue();
};

const storyURL = (id, theme) => {
	const url = new URL("iframe.html", base);
	url.searchParams.set("id", id);
	url.searchParams.set("viewMode", "story");
	if (theme) {
		url.searchParams.set("globals", `theme:${theme}`);
	}
	return url;
};

const { values, positionals } = parseArgs({
	allowPositionals: true,
	options: {
		out: { type: "string", short: "o" },
		theme: { type: "string", short: "t" },
	},
});
const [id] = positionals;
if (!id) {
	console.error(usage);
	process.exit(1);
}

const index = await fetch(new URL("index.json", base)).then((r) => r.json());
if (!index.entries[id]) {
	console.error(`Story not found: ${id}`);
	process.exit(1);
}

// pnpm runs scripts from site/, so resolve --out against the caller's cwd.
const out = values.out
	? resolve(process.env.INIT_CWD ?? process.cwd(), values.out)
	: join(tmpdir(), "storybook-screenshots", `${id}.png`);

const browser = await chromium.launch();
try {
	const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
	const event = await loadStory(page, storyURL(id, values.theme));
	if (event !== STORY_RENDERED) {
		console.error(`Story render failed (${event}); capturing anyway.`);
		process.exitCode = 1;
	}
	await page.evaluate(() => document.fonts.ready);
	await page.waitForLoadState("networkidle");
	await page.screenshot({ path: out });
	console.log(out);
} finally {
	await browser.close();
}
