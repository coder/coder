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
	FrontmatterError,
	parseFrontmatter,
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
	assert.equal(stripHtmlComments("a <!-- x --> b").content, "a  b");
	assert.equal(stripHtmlComments("<!-- a\nb -->\ntail").content, "\n\ntail");
	assert.equal(
		stripHtmlComments("```\n<!-- keep -->\n```").content,
		"```\n<!-- keep -->\n```",
	);
	// A comment inside a blockquoted fence is code, not a comment: left untouched.
	assert.equal(
		stripHtmlComments("> ```\n> <!-- keep -->\n> ```").content,
		"> ```\n> <!-- keep -->\n> ```",
	);
	// A comment inside an inline code span is code, not a comment: left untouched.
	assert.equal(
		stripHtmlComments("use `<!-- keep -->` inline").content,
		"use `<!-- keep -->` inline",
	);
	// A well-formed comment reports no unclosed opener.
	assert.equal(stripHtmlComments("a <!-- x --> b").unclosedCommentLine, null);
});

test("stripHtmlComments reports an unclosed comment by its opening line", () => {
	// The comment opens on line 5 and never closes, so it would otherwise swallow
	// the rest of the file; the 1-based opening line is returned so the caller can
	// fail and name it instead of shipping a truncated page.
	const { content, unclosedCommentLine } = stripHtmlComments(
		"# T\n\nFirst.\n\n<!-- unclosed\n\nrest\n",
	);
	assert.equal(unclosedCommentLine, 5);
	assert.equal(content.startsWith("# T\n\nFirst.\n\n"), true);
	assert.equal(content.includes("rest"), false);
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
	// A CommonMark angle-bracket link destination is left intact: rewriting the
	// inner autolink would nest brackets and ship a broken link. The bracketed
	// form is what wraps a URL containing parentheses (here a Wikipedia article).
	assert.equal(
		normalizeAngleBrackets(
			"[self-hosted](<https://en.wikipedia.org/wiki/Self-hosting_(web_services)>)",
		),
		"[self-hosted](<https://en.wikipedia.org/wiki/Self-hosting_(web_services)>)",
	);
	// A plain parenthetical autolink has no `]` before the `(`, so the
	// lookbehind does not fire and the autolink is still rewritten (the Maven
	// URL on the admin setup page relies on this).
	assert.equal(
		normalizeAngleBrackets("(<https://x.dev>)"),
		"([https://x.dev](https://x.dev))",
	);
	// The same lookbehind guards a bracketed email destination.
	assert.equal(normalizeAngleBrackets("[me](<a@b.com>)"), "[me](<a@b.com>)");
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
		frontmatterEnd: 0,
		description: null,
	});
	// Backticks are stripped from the title.
	assert.deepEqual(extractTitle("# `coder`", "x"), {
		title: "coder",
		h1Line: 0,
		frontmatterEnd: 0,
		description: null,
	});
	// Falls back to the manifest title when there is no H1.
	assert.deepEqual(
		extractTitle("no heading", "a/b", new Map([["a/b", { title: "Meta" }]])),
		{ title: "Meta", h1Line: -1, frontmatterEnd: 0, description: null },
	);
	// Falls back to a title-cased last segment when nothing else matches.
	assert.deepEqual(extractTitle("no heading", "guides/quick-start"), {
		title: "Quick Start",
		h1Line: -1,
		frontmatterEnd: 0,
		description: null,
	});
});

test("extractTitle reads frontmatter and ignores the make gen comment", () => {
	// make gen prepends `---`/`# Code generated ...`/`title:`/`---`. That comment
	// line matches the H1 scan, so without frontmatter handling the whole REST API
	// and CLI reference would be titled "Code generated ...". The title must come
	// from the frontmatter, and frontmatterEnd must cover the whole block so the
	// caller strips it (no leaked `---`/`title:` rendered in the body).
	const makeGen =
		"---\n# Code generated by make gen. DO NOT EDIT.\ntitle: General\n---\n\n## Section\n";
	assert.deepEqual(extractTitle(makeGen, "reference/api/general"), {
		title: "General",
		h1Line: -1,
		frontmatterEnd: 4,
		description: null,
	});
	// A frontmatter description is returned alongside the title.
	const withDesc = "---\ntitle: ssh\ndescription: Start a shell\n---\n\nBody\n";
	assert.deepEqual(extractTitle(withDesc, "reference/cli/ssh"), {
		title: "ssh",
		h1Line: -1,
		frontmatterEnd: 4,
		description: "Start a shell",
	});
	// A frontmatter title wins over a body H1, and the H1 is still located (in
	// original coordinates) so the caller strips the duplicate.
	const both = "---\ntitle: Front\n---\n\n# Body H1\n\ntext";
	assert.deepEqual(extractTitle(both, "x"), {
		title: "Front",
		h1Line: 4,
		frontmatterEnd: 3,
		description: null,
	});
});

test("parseFrontmatter reads a flat block and ignores non-frontmatter", () => {
	// A make gen block: the comment line is skipped, title is read, and endLine is
	// the first body line past the closing ---.
	assert.deepEqual(
		parseFrontmatter(
			"---\n# Code generated by make gen. DO NOT EDIT.\ntitle: General\n---\nbody",
		),
		{ title: "General", description: null, endLine: 4 },
	);
	// Surrounding quotes on a value are stripped.
	assert.deepEqual(parseFrontmatter('---\ntitle: "A B"\n---\n'), {
		title: "A B",
		description: null,
		endLine: 3,
	});
	// A block with a nested YAML value (make gen's `state:` list on early-access
	// pages) is a valid mapping, so it is frontmatter: title/description are read
	// and endLine points past the whole block, including the nested lines.
	assert.deepEqual(
		parseFrontmatter(
			"---\n# Code generated by make gen. DO NOT EDIT.\ntitle: Chats\ndescription: X\nstate:\n  - early access\n---\nbody",
		),
		{ title: "Chats", description: "X", endLine: 7 },
	);
	// No leading --- means no frontmatter.
	assert.deepEqual(parseFrontmatter("# Title\n\nbody"), {
		title: null,
		description: null,
		endLine: 0,
	});
	// A --- block whose body is not flat key/value is content (a thematic rule),
	// not frontmatter, so nothing is stripped.
	assert.deepEqual(parseFrontmatter("---\njust prose\n---\n"), {
		title: null,
		description: null,
		endLine: 0,
	});
	// An unterminated block is not frontmatter.
	assert.deepEqual(parseFrontmatter("---\ntitle: X\n"), {
		title: null,
		description: null,
		endLine: 0,
	});
});

test("parseFrontmatter decodes YAML escapes and only treats mappings as frontmatter", () => {
	// A quoted description with YAML escapes (make gen emits these for CLI pages
	// that mention `"ssh ..."`) decodes to real inner quotes, not a literal
	// backslash-quote leaking into <meta> and the search index.
	assert.deepEqual(
		parseFrontmatter(
			'---\ntitle: ssh\ndescription: "Add a host \\"ssh x\\""\n---\nbody',
		),
		{ title: "ssh", description: 'Add a host "ssh x"', endLine: 4 },
	);
	// Extra, unrecognized keys are fine: the block is still a YAML mapping, so it
	// is frontmatter and the title/description are read from it.
	assert.deepEqual(parseFrontmatter("---\nfoo: bar\ntitle: T\n---\nbody"), {
		title: "T",
		description: null,
		endLine: 4,
	});
	// A non-string scalar for title/description (never in the corpus) is treated
	// as absent so the result stays a plain string or null.
	assert.deepEqual(parseFrontmatter("---\ntitle: 123\n---\nbody"), {
		title: null,
		description: null,
		endLine: 3,
	});
});

test("parseFrontmatter throws a labeled FrontmatterError on invalid YAML", () => {
	// A `---` block that opens and closes but whose body is not valid YAML (here
	// an unterminated double-quoted scalar) throws a FrontmatterError carrying the
	// js-yaml reason, so the sync can name the offending source file instead of
	// crashing with an opaque parser stack trace.
	assert.throws(
		() => parseFrontmatter('---\ntitle: "unterminated\n---\nbody'),
		(err) =>
			err instanceof FrontmatterError &&
			err.name === "FrontmatterError" &&
			typeof err.reason === "string" &&
			err.reason.length > 0,
	);
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

test("rewriteTarget points source-tree links at GitHub via ctx.sourceLink", () => {
	const ctx = {
		imageRemote: "",
		resolveMd: () => null,
		copyImage: () => null,
		sourceLink: (repoRel) => `https://gh/${repoRel}`,
	};
	// A non-image, non-.md target that escapes docs/ resolves against the repo
	// root and is handed to sourceLink.
	assert.equal(
		rewriteTarget("../../coderd", "install/x.md", ctx),
		"https://gh/coderd",
	);
	// The anchor survives the rewrite.
	assert.equal(
		rewriteTarget("../../coderd/database#schema", "install/x.md", ctx),
		"https://gh/coderd/database#schema",
	);
	// Images that escape docs/ are not rewritten (they would need bundling).
	assert.equal(rewriteTarget("../../logo.png", "install/x.md", ctx), null);
	// With no sourceLink in ctx, an escaping target is left unchanged.
	assert.equal(
		rewriteTarget("../../coderd", "install/x.md", {
			imageRemote: "",
			resolveMd: () => null,
			copyImage: () => null,
		}),
		null,
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
	// A fence nested in a blockquote is code too: its link text is not rewritten.
	assert.equal(
		rewriteContent("> ```\n> see [x](b.md)\n> ```", "a.md", ctx),
		"> ```\n> see [x](b.md)\n> ```",
	);
	// A link inside an inline code span is left verbatim (not rewritten), so a
	// Markdown example that shows `[x](b.md)` is not turned into a live route.
	assert.equal(
		rewriteContent("see `[x](b.md)` inline", "a.md", ctx),
		"see `[x](b.md)` inline",
	);
	// An href inside an inline code span is left verbatim too.
	assert.equal(
		rewriteContent('write `<a href="b.md">` here', "a.md", ctx),
		'write `<a href="b.md">` here',
	);
	// A plain blockquote (no fence) is prose, so its link is still rewritten.
	assert.equal(rewriteContent("> see [x](b.md)", "a.md", ctx), "> see [x](/b)");
});
