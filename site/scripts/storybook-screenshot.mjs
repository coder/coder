// Screenshot a story from a running Storybook dev server.
//
// Usage: pnpm storybook:screenshot <story> [options]
//
// <story> is a story ID (components-button--default), a Storybook URL, or a
// "Title/Path/Story Name" pair.
import { mkdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { chromium } from "@playwright/test";
import { toId } from "storybook/internal/csf";

const usage = `Usage: pnpm storybook:screenshot <story> [options]

<story>  Story ID, Storybook URL, or "Title/Path/Story Name".

Options:
  -o, --out <file>     Output path (default: $TMPDIR/storybook-screenshots/<id>.png)
  -t, --theme <name>   Storybook theme global, e.g. light or dark
  -w, --width <px>     Viewport width (default: 1280)
  -h, --height <px>    Viewport height (default: 800)
  -f, --full-page      Capture the full scrollable page
  -s, --selector <css> Capture only the first matching element
  -u, --url <url>      Storybook base URL (default: $STORYBOOK_URL or http://localhost:6006)
      --timeout <ms>   Render timeout (default: 30000)
      --help           Show this message`;

const { values, positionals } = parseArgs({
	allowPositionals: true,
	options: {
		out: { type: "string", short: "o" },
		theme: { type: "string", short: "t" },
		width: { type: "string", short: "w", default: "1280" },
		height: { type: "string", short: "h", default: "800" },
		"full-page": { type: "boolean", short: "f", default: false },
		selector: { type: "string", short: "s" },
		url: {
			type: "string",
			short: "u",
			default: process.env.STORYBOOK_URL ?? "http://localhost:6006",
		},
		timeout: { type: "string", default: "30000" },
		help: { type: "boolean", default: false },
	},
});

if (values.help || positionals.length !== 1) {
	console.log(usage);
	process.exit(values.help ? 0 : 1);
}

// Trailing slash so relative paths resolve under a Storybook mounted on a subpath.
const baseURL = new URL(values.url);
if (!baseURL.pathname.endsWith("/")) {
	baseURL.pathname += "/";
}
const timeout = Number(values.timeout);

const toStoryId = (input) => {
	if (/^https?:\/\//.test(input)) {
		const url = new URL(input);
		const id =
			url.searchParams.get("id") ??
			url.searchParams.get("path")?.replace(/^\/(story|docs)\//, "");
		if (!id) {
			throw new Error(`No story ID found in URL: ${input}`);
		}
		return id;
	}
	if (input.includes("/")) {
		const parts = input.split("/");
		const name = parts.pop();
		return toId(parts.join("/"), name);
	}
	return input;
};

const storyId = toStoryId(positionals[0]);

const index = await fetch(new URL("index.json", baseURL))
	.then((res) => res.json())
	.catch(() => {
		console.error(
			`Storybook is not reachable at ${baseURL}. Start it with "pnpm storybook".`,
		);
		process.exit(1);
	});

if (!index.entries[storyId]) {
	console.error(`Story not found: ${storyId}`);
	process.exit(1);
}

const storyURL = new URL("iframe.html", baseURL);
storyURL.searchParams.set("id", storyId);
storyURL.searchParams.set("viewMode", "story");
if (values.theme) {
	storyURL.searchParams.set("globals", `theme:${values.theme}`);
}
// pnpm runs scripts from site/, so resolve --out against the caller's cwd.
const out = values.out
	? resolve(process.env.INIT_CWD ?? process.cwd(), values.out)
	: join(tmpdir(), "storybook-screenshots", `${storyId}.png`);

const browser = await chromium.launch();
try {
	const page = await browser.newPage({
		viewport: { width: Number(values.width), height: Number(values.height) },
	});
	await page.goto(storyURL.href);

	const phase = await page
		.waitForFunction(
			() => {
				const p = globalThis.__STORYBOOK_PREVIEW__?.currentRender?.phase;
				return ["finished", "errored", "aborted"].includes(p) && p;
			},
			undefined,
			{ timeout },
		)
		.then((handle) => handle.jsonValue());
	if (phase !== "finished") {
		console.error(`Story render ${phase}; capturing anyway.`);
		process.exitCode = 1;
	}

	await page.evaluate(() => document.fonts.ready);
	await page.waitForLoadState("networkidle");

	await mkdir(dirname(out), { recursive: true });
	if (values.selector) {
		await page.locator(values.selector).first().screenshot({ path: out });
	} else {
		await page.screenshot({ path: out, fullPage: values["full-page"] });
	}
	console.log(out);
} finally {
	await browser.close();
}
