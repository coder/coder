import { afterEach, describe, expect, it, vi } from "vitest";
import { parseDiffString } from "./parseDiff";
import {
	collectRenderedSegments,
	isWithinRenderBudget,
	MAX_RENDERED_MARKDOWN_CHARS,
	renderedFileDiff,
	renderedMarkdownUrlTransform,
} from "./renderedMarkdown";

function parseSingle(diff: string) {
	const [file] = parseDiffString(diff);
	if (!file) {
		throw new Error("expected one parsed file");
	}
	return file;
}

const newFileDiff = [
	"diff --git a/README.md b/README.md",
	"new file mode 100644",
	"index 0000000..1111111",
	"--- /dev/null",
	"+++ b/README.md",
	"@@ -0,0 +1,3 @@",
	"+# Title",
	"+",
	"+Body text.",
].join("\n");

const modifiedFileDiff = [
	"diff --git a/docs/guide.md b/docs/guide.md",
	"index 1111111..2222222 100644",
	"--- a/docs/guide.md",
	"+++ b/docs/guide.md",
	"@@ -1,4 +1,4 @@",
	" # Guide",
	"-Old intro",
	"+New intro",
	" ",
	" Unchanged",
	"@@ -20,3 +20,4 @@",
	" ## Section",
	"-removed line",
	"+added line",
	"+another added line",
	" trailing",
].join("\n");

const deletedFileDiff = [
	"diff --git a/OLD.md b/OLD.md",
	"deleted file mode 100644",
	"index 1111111..0000000",
	"--- a/OLD.md",
	"+++ /dev/null",
	"@@ -1,2 +0,0 @@",
	"-# Gone",
	"-Removed body",
].join("\n");

const pureRenameDiff = [
	"diff --git a/a.md b/b.md",
	"similarity index 100%",
	"rename from a.md",
	"rename to b.md",
].join("\n");

describe("collectRenderedSegments", () => {
	it("renders a new file as one complete segment", () => {
		const result = collectRenderedSegments(parseSingle(newFileDiff));
		expect(result.side).toBe("additions");
		expect(result.isComplete).toBe(true);
		expect(result.segments).toEqual([
			{ startLine: 1, endLine: 3, text: "# Title\n\nBody text." },
		]);
		expect(result.totalChars).toBe("# Title\n\nBody text.".length);
	});

	it("renders one new-side segment per hunk without deleted lines", () => {
		const result = collectRenderedSegments(parseSingle(modifiedFileDiff));
		expect(result.side).toBe("additions");
		expect(result.isComplete).toBe(false);
		expect(result.segments).toEqual([
			{
				startLine: 1,
				endLine: 4,
				text: "# Guide\nNew intro\n\nUnchanged",
			},
			{
				startLine: 20,
				endLine: 23,
				text: "## Section\nadded line\nanother added line\ntrailing",
			},
		]);
		const joined = result.segments.map((s) => s.text).join("\n");
		expect(joined).not.toContain("Old intro");
		expect(joined).not.toContain("removed line");
		expect(result.totalChars).toBe(
			result.segments.reduce((sum, s) => sum + s.text.length, 0),
		);
	});

	it("renders the old side of a deleted file", () => {
		const result = collectRenderedSegments(parseSingle(deletedFileDiff));
		expect(result.side).toBe("deletions");
		expect(result.isComplete).toBe(true);
		expect(result.segments).toEqual([
			{ startLine: 1, endLine: 2, text: "# Gone\nRemoved body" },
		]);
	});

	it("yields no segments for a pure rename", () => {
		const result = collectRenderedSegments(parseSingle(pureRenameDiff));
		expect(result.segments).toEqual([]);
		expect(result.totalChars).toBe(0);
	});
});

describe("isWithinRenderBudget", () => {
	it("accepts text up to the cap and rejects text above it", () => {
		const base = collectRenderedSegments(parseSingle(newFileDiff));
		expect(
			isWithinRenderBudget({
				...base,
				totalChars: MAX_RENDERED_MARKDOWN_CHARS,
			}),
		).toBe(true);
		expect(
			isWithinRenderBudget({
				...base,
				totalChars: MAX_RENDERED_MARKDOWN_CHARS + 1,
			}),
		).toBe(false);
	});
});

describe("renderedFileDiff", () => {
	it("empties the hunks, retypes changes as new, and rekeys the cache", () => {
		const source = parseSingle(modifiedFileDiff);
		const result = renderedFileDiff(source);
		expect(result.name).toBe(source.name);
		expect(result.type).toBe("new");
		expect(result.hunks).toEqual([]);
		expect(result.additionLines).toEqual([]);
		expect(result.deletionLines).toEqual([]);
		expect(result.splitLineCount).toBe(0);
		expect(result.unifiedLineCount).toBe(0);
		expect(result.cacheKey).toBe(`${source.cacheKey}:rendered`);
		expect(source.hunks.length).toBe(2);
	});

	it("keeps the deleted type so the old side stays renderable", () => {
		expect(renderedFileDiff(parseSingle(deletedFileDiff)).type).toBe("deleted");
	});
});

describe("renderedMarkdownUrlTransform", () => {
	const node: Parameters<typeof renderedMarkdownUrlTransform>[2] = {
		type: "element",
		tagName: "a",
		properties: {},
		children: [],
	};
	// Assembled at runtime so the literal does not trip oxlint's
	// no-script-url rule, which cannot tell test data from executable code.
	const scriptUrl = ["javascript", "alert(1)"].join(":");
	const transform = (url: string, key: string) =>
		renderedMarkdownUrlTransform(url, key, node);

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it.each([
		["https://example.com/docs", "https://example.com/docs"],
		["http://example.com/docs", "http://example.com/docs"],
		["mailto:team@example.com", "mailto:team@example.com"],
		["docs/guide.md", null],
		["./guide.md", null],
		["/absolute/path.md", null],
		["#section", null],
		["//example.com/path", null],
		[scriptUrl, null],
		["file:///etc/passwd", null],
		["tel:+15555550100", null],
	])("maps href %s to %s", (url, expected) => {
		expect(transform(url, "href")).toBe(expected);
	});

	it("keeps image sources on other hosts", () => {
		vi.stubGlobal("location", {
			origin: "https://coder.example.com",
			hostname: "coder.example.com",
		});
		expect(transform("https://cdn.example.com/a.png", "src")).toBe(
			"https://cdn.example.com/a.png",
		);
		expect(transform("http://cdn.example.com/a.png", "src")).toBe(
			"http://cdn.example.com/a.png",
		);
	});

	it.each([
		["https://coder.example.com/@owner/ws/apps/x/p.gif"],
		["https://coder.example.com/api/v2/users/me"],
		["http://coder.example.com/@owner/ws/apps/x/p.gif"],
		["https://coder.example.com:8443/@owner/ws/apps/x/p.gif"],
		["https://coder.example.com./@owner/ws/apps/x/p.gif"],
		["https://user:pass@coder.example.com/p.gif"],
		["assets/logo.png"],
		["./logo.png"],
		["/logo.png"],
		["//cdn.example.com/a.png"],
		["data:image/png;base64,AAAA"],
		["blob:https://coder.example.com/1234"],
		[scriptUrl],
	])("drops image source %s", (url) => {
		vi.stubGlobal("location", {
			origin: "https://coder.example.com",
			hostname: "coder.example.com",
		});
		expect(transform(url, "src")).toBeNull();
	});

	it("drops URLs on attributes other than href and src", () => {
		expect(transform("https://example.com", "cite")).toBeNull();
	});
});
