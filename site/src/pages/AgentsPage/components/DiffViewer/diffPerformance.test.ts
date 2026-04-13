import type { FileDiffMetadata } from "@pierre/diffs";
import { describe, expect, it } from "vitest";
import { getDiffRenderMode, isSafariUserAgent } from "./diffPerformance";

function createFile(name: string, unifiedLineCount: number): FileDiffMetadata {
	return {
		name,
		type: "change",
		hunks: [],
		splitLineCount: unifiedLineCount,
		unifiedLineCount,
		isPartial: true,
		deletionLines: [],
		additionLines: [],
	};
}

describe("isSafariUserAgent", () => {
	it("detects desktop Safari", () => {
		expect(
			isSafariUserAgent(
				"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
			),
		).toBe(true);
	});

	it("detects iOS WebKit browsers", () => {
		expect(
			isSafariUserAgent(
				"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/123.0.0.0 Mobile/15E148 Safari/604.1",
			),
		).toBe(true);
	});

	it("does not match non-WebKit browsers", () => {
		expect(
			isSafariUserAgent(
				"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
			),
		).toBe(false);
	});
});

describe("getDiffRenderMode", () => {
	it("uses the default wrapped mode for smaller diffs", () => {
		const mode = getDiffRenderMode([createFile("small.ts", 200)], "");
		expect(mode.isLargeDiff).toBe(false);
		expect(mode.overflow).toBe("wrap");
		expect(mode.showStickyHeaders).toBe(true);
		expect(mode.enableTreeSync).toBe(true);
		expect(mode.scrollBehavior).toBe("smooth");
		expect(mode.virtualizerConfig).toEqual({
			overscrollSize: 600,
			intersectionObserverMargin: 1600,
		});
	});

	it("switches to simplified mode for large file counts", () => {
		const files = Array.from({ length: 24 }, (_, index) =>
			createFile(`file-${index}.ts`, 20),
		);
		const mode = getDiffRenderMode(files, "");
		expect(mode.isLargeDiff).toBe(true);
		expect(mode.overflow).toBe("scroll");
		expect(mode.showStickyHeaders).toBe(false);
		expect(mode.enableTreeSync).toBe(false);
		expect(mode.scrollBehavior).toBe("instant");
		expect(mode.virtualizerConfig).toEqual({
			overscrollSize: 300,
			intersectionObserverMargin: 900,
		});
	});

	it("switches to simplified mode for large total line counts", () => {
		const mode = getDiffRenderMode(
			[createFile("big.ts", 1499), createFile("another.ts", 10)],
			"",
		);
		expect(mode.isLargeDiff).toBe(true);
		expect(mode.overflow).toBe("scroll");
	});

	it("switches to simplified mode for Safari even on smaller diffs", () => {
		const mode = getDiffRenderMode(
			[createFile("small.ts", 120)],
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
		);
		expect(mode.isSafari).toBe(true);
		expect(mode.overflow).toBe("scroll");
		expect(mode.enableTreeSync).toBe(false);
	});
});
