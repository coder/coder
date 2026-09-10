import { describe, expect, it } from "vitest";
import {
	buildDiffViewerItems,
	PREVIEW_BLOCKED_BY_COMMENT_REASON,
	PREVIEW_TOO_LARGE_REASON,
	resolveMarkdownPreviews,
	toggleRenderedFile,
} from "./diffViewerItems";
import { parseDiffString } from "./parseDiff";
import {
	MAX_RENDERED_MARKDOWN_CHARS,
	RENDERED_MARKDOWN_ANNOTATION,
} from "./renderedMarkdown";
import { generateNewFileDiff } from "./testHelpers";

const pureRename = [
	"diff --git a/a.md b/b.md",
	"similarity index 100%",
	"rename from a.md",
	"rename to b.md",
].join("\n");

const files = parseDiffString(
	[
		generateNewFileDiff("docs/README.md", ["# Title", "Body"]),
		generateNewFileDiff("src/main.ts", ["const a = 1;"]),
		generateNewFileDiff("docs/HUGE.md", [
			"a".repeat(MAX_RENDERED_MARKDOWN_CHARS + 1),
		]),
		pureRename,
	].join("\n"),
);
const noComments = () => false;

describe("resolveMarkdownPreviews", () => {
	it("tracks Markdown files with renderable content only", () => {
		const previews = resolveMarkdownPreviews(files, new Set(), noComments);
		expect([...previews.keys()].sort()).toEqual([
			"docs/HUGE.md",
			"docs/README.md",
		]);
	});

	it("reports a toggled file as rendered", () => {
		const previews = resolveMarkdownPreviews(
			files,
			new Set(["docs/README.md"]),
			noComments,
		);
		expect(previews.get("docs/README.md")).toEqual({
			isRendered: true,
			disabledReason: undefined,
		});
	});

	it("never renders a file over budget, even when toggled", () => {
		const previews = resolveMarkdownPreviews(
			files,
			new Set(["docs/HUGE.md"]),
			noComments,
		);
		expect(previews.get("docs/HUGE.md")).toEqual({
			isRendered: false,
			disabledReason: PREVIEW_TOO_LARGE_REASON,
		});
	});

	it("blocks toggling on while a comment is open and allows toggling off", () => {
		const hasComment = (name: string) => name === "docs/README.md";
		expect(
			resolveMarkdownPreviews(files, new Set(), hasComment).get(
				"docs/README.md",
			),
		).toEqual({
			isRendered: false,
			disabledReason: PREVIEW_BLOCKED_BY_COMMENT_REASON,
		});
		expect(
			resolveMarkdownPreviews(
				files,
				new Set(["docs/README.md"]),
				hasComment,
			).get("docs/README.md"),
		).toEqual({ isRendered: true, disabledReason: undefined });
	});
});

describe("toggleRenderedFile", () => {
	it("adds a missing file and removes a present one without mutating", () => {
		const initial: ReadonlySet<string> = new Set(["a.md"]);
		const added = toggleRenderedFile(initial, "b.md");
		expect([...added].sort()).toEqual(["a.md", "b.md"]);
		const removed = toggleRenderedFile(added, "a.md");
		expect([...removed]).toEqual(["b.md"]);
		expect([...initial]).toEqual(["a.md"]);
	});
});

describe("buildDiffViewerItems", () => {
	const readme = files.find((f) => f.name === "docs/README.md");
	const main = files.find((f) => f.name === "src/main.ts");
	if (!readme || !main) {
		throw new Error("fixture files missing");
	}

	it("keeps every item raw when nothing is previewed", () => {
		const items = buildDiffViewerItems(
			[readme, main],
			resolveMarkdownPreviews([readme, main], new Set(), noComments),
		);
		expect(items.map((item) => item.id)).toEqual([
			"docs/README.md",
			"src/main.ts",
		]);
		for (const item of items) {
			expect(item.type).toBe("diff");
			expect(item.version).toBe(0);
		}
		expect(items[0].type === "diff" && items[0].fileDiff).toBe(readme);
	});

	it("swaps only the previewed file for the stand-in and leaves neighbours untouched", () => {
		const rendered = new Set(["docs/README.md"]);
		const items = buildDiffViewerItems(
			[readme, main],
			resolveMarkdownPreviews([readme, main], rendered, noComments),
		);
		const [readmeItem, mainItem] = items;
		if (readmeItem.type !== "diff" || mainItem.type !== "diff") {
			throw new Error("expected diff items");
		}
		expect(readmeItem.fileDiff.hunks).toEqual([]);
		expect(readmeItem.fileDiff.cacheKey).toBe(`${readme.cacheKey}:rendered`);
		expect(readmeItem.annotations).toEqual([
			{
				side: "additions",
				lineNumber: 0,
				metadata: RENDERED_MARKDOWN_ANNOTATION,
			},
		]);
		expect(readmeItem.version).toBe(1);
		expect(mainItem.fileDiff).toBe(main);
		expect(mainItem.annotations).toBeUndefined();
		expect(mainItem.version).toBe(0);
	});

	it("changes the version on every toggle so CodeView re-reads the item", () => {
		const versionFor = (rendered: ReadonlySet<string>) => {
			const [item] = buildDiffViewerItems(
				[readme],
				resolveMarkdownPreviews([readme], rendered, noComments),
			);
			return item.version;
		};
		const off = versionFor(new Set());
		const on = versionFor(new Set(["docs/README.md"]));
		expect(on).not.toBe(off);
		expect(versionFor(new Set())).toBe(off);
	});

	it("passes line annotations through for raw files", () => {
		const annotation = {
			side: "additions" as const,
			lineNumber: 2,
			metadata: "active-input",
		};
		const [item] = buildDiffViewerItems(
			[readme],
			resolveMarkdownPreviews([readme], new Set(), () => true),
			() => [annotation],
		);
		expect(item.annotations).toEqual([annotation]);
		expect(item.version).not.toBe(0);
	});
});
