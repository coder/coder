import test from "node:test";
import assert from "node:assert/strict";

import {
	slugSegment,
	isIndexFile,
	normalizeManifestPath,
	titleCase,
	lastSeg,
	normalizeFences,
	normalizeStepHeadings,
	stripHtmlComments,
	normalizeAngleBrackets,
	escapeCurlyBraces,
	selfCloseVoidElements,
	extractTitle,
	rewriteTarget,
	rewriteContent,
} from "./transform.mjs";

test("slugSegment lowercases, strips .md, and dashes non-alphanumerics", () => {
	assert.equal(slugSegment("Getting-Started.md"), "getting-started");
	assert.equal(slugSegment("A B/C"), "a-b-c");
	assert.equal(slugSegment("---"), "page");
	assert.equal(slugSegment(""), "page");
});

test("isIndexFile matches index/readme case-insensitively", () => {
	assert.equal(isIndexFile("index.md"), true);
	assert.equal(isIndexFile("README.md"), true);
	assert.equal(isIndexFile("Readme.md"), true);
	assert.equal(isIndexFile("guide.md"), false);
});

test("normalizeManifestPath strips a leading ./ or /", () => {
	assert.equal(normalizeManifestPath("./a/b.md"), "a/b.md");
	assert.equal(normalizeManifestPath("/a/b.md"), "a/b.md");
	assert.equal(normalizeManifestPath(""), "");
	assert.equal(normalizeManifestPath(undefined), "");
});

test("titleCase converts separators and capitalizes words", () => {
	assert.equal(titleCase("getting-started"), "Getting Started");
	assert.equal(titleCase("a_b-c"), "A B C");
});

test("lastSeg returns the final path segment", () => {
	assert.equal(lastSeg("a/b/c"), "c");
	assert.equal(lastSeg("solo"), "solo");
});

test("normalizeFences maps language aliases and lowercases the info string", () => {
	assert.equal(normalizeFences("```ENV\nx\n```"), "```ini\nx\n```");
	assert.equal(normalizeFences("```pwsh\n```"), "```powershell\n```");
	assert.equal(normalizeFences("```JS\n```"), "```js\n```");
	// A closing fence and the fenced content are left untouched.
	assert.equal(normalizeFences("```\nENV\n```"), "```\nENV\n```");
	// Tilde fences are honored too.
	assert.equal(normalizeFences("~~~Bash\n~~~"), "~~~bash\n~~~");
});

test("normalizeStepHeadings rewrites 'Step N:' headings and skips fences", () => {
	assert.equal(
		normalizeStepHeadings("## Step 1: Install"),
		"## Install [step]",
	);
	assert.equal(normalizeStepHeadings("### step 2: Do it"), "### Do it [step]");
	assert.equal(
		normalizeStepHeadings("```\n## Step 1: Nope\n```"),
		"```\n## Step 1: Nope\n```",
	);
});

test("stripHtmlComments removes comments, preserves line count, keeps fenced", () => {
	assert.equal(stripHtmlComments("a <!-- x --> b"), "a  b");
	assert.equal(stripHtmlComments("<!-- a\nb -->\ntail"), "\n\ntail");
	assert.equal(
		stripHtmlComments("```\n<!-- keep -->\n```"),
		"```\n<!-- keep -->\n```",
	);
});

test("normalizeAngleBrackets rewrites autolinks and escapes stray <", () => {
	assert.equal(
		normalizeAngleBrackets("<https://x.dev>"),
		"[https://x.dev](https://x.dev)",
	);
	assert.equal(
		normalizeAngleBrackets("<a@b.com>"),
		"[a@b.com](mailto:a@b.com)",
	);
	assert.equal(normalizeAngleBrackets("a < b"), "a \\< b");
	// Real tags and inline code are left untouched.
	assert.equal(normalizeAngleBrackets("<div>"), "<div>");
	assert.equal(normalizeAngleBrackets("`a < b`"), "`a < b`");
});

test("escapeCurlyBraces escapes braces in prose but not code", () => {
	assert.equal(escapeCurlyBraces("use {id} here"), "use \\{id\\} here");
	assert.equal(escapeCurlyBraces("`{id}`"), "`{id}`");
	assert.equal(escapeCurlyBraces("```\n{id}\n```"), "```\n{id}\n```");
});

test("selfCloseVoidElements self-closes void tags outside code", () => {
	assert.equal(selfCloseVoidElements("a<br>b"), "a<br />b");
	assert.equal(selfCloseVoidElements('<img src="x">'), '<img src="x" />');
	// Already self-closed and inline code are left byte-identical.
	assert.equal(selfCloseVoidElements("<br/>"), "<br/>");
	assert.equal(selfCloseVoidElements("`<br>`"), "`<br>`");
});

test("extractTitle finds the first H1, else falls back", () => {
	assert.deepEqual(extractTitle("# Hello\n\nbody", "a/b"), {
		title: "Hello",
		h1Line: 0,
	});
	// Backticks are stripped from the title.
	assert.deepEqual(extractTitle("# `coder`", "x"), {
		title: "coder",
		h1Line: 0,
	});
	// Falls back to the manifest title when there is no H1.
	assert.deepEqual(
		extractTitle("no heading", "a/b", new Map([["a/b", { title: "Meta" }]])),
		{ title: "Meta", h1Line: -1 },
	);
	// Falls back to a title-cased last segment when nothing else matches.
	assert.deepEqual(extractTitle("no heading", "guides/quick-start"), {
		title: "Quick Start",
		h1Line: -1,
	});
});

test("rewriteTarget resolves .md links and images through ctx", () => {
	const ctx = {
		imageRemote: "",
		resolveMd: (rel) => (rel === "guide.md" ? "/guide" : null),
		copyImage: (rel) => `/assets/${rel}`,
	};
	// External links and bare anchors are left unchanged.
	assert.equal(rewriteTarget("https://x.dev", "b.md", ctx), null);
	assert.equal(rewriteTarget("#frag", "b.md", ctx), null);
	// A sibling .md resolves to its route and keeps the anchor.
	assert.equal(rewriteTarget("guide.md#sec", "b.md", ctx), "/guide#sec");
	// An unmapped .md returns null so the caller leaves it verbatim.
	assert.equal(rewriteTarget("missing.md", "b.md", ctx), null);
	// A local image is copied through ctx.copyImage.
	assert.equal(rewriteTarget("img.png", "b.md", ctx), "/assets/img.png");
	// A target that escapes the corpus root is refused.
	assert.equal(rewriteTarget("../../x.md", "b.md", ctx), null);
});

test("rewriteTarget uses the remote image base when configured", () => {
	const ctx = {
		imageRemote: "https://cdn.example/img",
		resolveMd: () => null,
		copyImage: () => null,
	};
	assert.equal(
		rewriteTarget("shot.png", "b.md", ctx),
		"https://cdn.example/img/shot.png",
	);
});

test("rewriteContent rewrites markdown links and html attrs, skips fences", () => {
	const ctx = {
		imageRemote: "",
		resolveMd: (rel) => (rel === "b.md" ? "/b" : null),
		copyImage: () => null,
	};
	assert.equal(rewriteContent("see [x](b.md)", "a.md", ctx), "see [x](/b)");
	assert.equal(
		rewriteContent('<a href="b.md">x</a>', "a.md", ctx),
		'<a href="/b">x</a>',
	);
	// Links inside fenced code are left verbatim.
	assert.equal(
		rewriteContent("```\n[x](b.md)\n```", "a.md", ctx),
		"```\n[x](b.md)\n```",
	);
});
