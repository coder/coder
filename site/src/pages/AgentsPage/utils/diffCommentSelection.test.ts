import { describe, expect, it } from "vitest";
import {
	annotationLineForBox,
	annotationSideForBox,
	type CommentBoxState,
	commentBoxFromRange,
	contentRangeForBox,
	selectedLinesForBox,
} from "./diffCommentSelection";

const FILE = "src/main.ts";

describe("commentBoxFromRange", () => {
	it("returns null when the range is null (selection cleared)", () => {
		expect(commentBoxFromRange(FILE, null)).toBeNull();
	});

	it("ignores same-side single-line selections (handled by line number click)", () => {
		expect(
			commentBoxFromRange(FILE, {
				start: 10,
				end: 10,
				side: "additions",
			}),
		).toBe("ignore");
	});

	it("ignores single-line selections with no side at all", () => {
		expect(commentBoxFromRange(FILE, { start: 5, end: 5 })).toBe("ignore");
	});

	it("allows cross-side selections even when start === end", () => {
		const result = commentBoxFromRange(FILE, {
			start: 16,
			end: 16,
			side: "additions",
			endSide: "deletions",
		});
		expect(result).toEqual({
			fileName: FILE,
			start: 16,
			startSide: "additions",
			end: 16,
			endSide: "deletions",
		});
	});

	it("creates a comment box for a multi-line same-side selection", () => {
		const result = commentBoxFromRange(FILE, {
			start: 10,
			end: 14,
			side: "deletions",
		});
		expect(result).toEqual({
			fileName: FILE,
			start: 10,
			startSide: "deletions",
			end: 14,
			endSide: "deletions",
		});
	});

	it("creates a comment box for a multi-line cross-side selection", () => {
		const result = commentBoxFromRange(FILE, {
			start: 509,
			end: 219,
			side: "deletions",
			endSide: "additions",
		});
		expect(result).toEqual({
			fileName: FILE,
			start: 509,
			startSide: "deletions",
			end: 219,
			endSide: "additions",
		});
	});

	it("preserves raw start/end without min/max normalization", () => {
		const result = commentBoxFromRange(FILE, {
			start: 20,
			end: 15,
			side: "additions",
		});
		expect(result).toMatchObject({ start: 20, end: 15 });
	});

	it("defaults side to additions when omitted", () => {
		const result = commentBoxFromRange(FILE, { start: 3, end: 7 });
		expect(result).toMatchObject({
			startSide: "additions",
			endSide: "additions",
		});
	});
});

describe("annotationSideForBox", () => {
	it("returns endSide (same as startSide for same-side selections)", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 1,
			startSide: "deletions",
			end: 5,
			endSide: "deletions",
		};
		expect(annotationSideForBox(box)).toBe("deletions");
	});

	it("returns endSide for cross-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 509,
			startSide: "deletions",
			end: 219,
			endSide: "additions",
		};
		expect(annotationSideForBox(box)).toBe("additions");
	});
});

describe("annotationLineForBox", () => {
	it("returns the end line number", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 509,
			startSide: "deletions",
			end: 219,
			endSide: "additions",
		};
		expect(annotationLineForBox(box)).toBe(219);
	});
});

describe("selectedLinesForBox", () => {
	it("builds a range without endSide for same-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 3,
			startSide: "additions",
			end: 8,
			endSide: "additions",
		};
		const range = selectedLinesForBox(box);
		expect(range).toEqual({
			start: 3,
			end: 8,
			side: "additions",
		});
		expect(range).not.toHaveProperty("endSide");
	});

	it("includes endSide for cross-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 509,
			startSide: "deletions",
			end: 219,
			endSide: "additions",
		};
		expect(selectedLinesForBox(box)).toEqual({
			start: 509,
			end: 219,
			side: "deletions",
			endSide: "additions",
		});
	});
});

describe("contentRangeForBox", () => {
	it("normalizes direction for same-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 20,
			startSide: "additions",
			end: 15,
			endSide: "additions",
		};
		expect(contentRangeForBox(box)).toEqual({
			startLine: 15,
			endLine: 20,
			side: "additions",
		});
	});

	it("uses the end point only for cross-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 509,
			startSide: "deletions",
			end: 219,
			endSide: "additions",
		};
		expect(contentRangeForBox(box)).toEqual({
			startLine: 219,
			endLine: 219,
			side: "additions",
		});
	});
});

// -------------------------------------------------------------------
// Edge cases
// -------------------------------------------------------------------

describe("edge cases", () => {
	// -- commentBoxFromRange ------------------------------------------

	it("line 1 selection is not ignored (start !== end)", () => {
		// Selecting lines 1-2 on the very first line of a file.
		const result = commentBoxFromRange(FILE, {
			start: 1,
			end: 2,
			side: "additions",
		});
		expect(result).toMatchObject({ start: 1, end: 2 });
	});

	it("single-line deletion-side click is ignored", () => {
		expect(
			commentBoxFromRange(FILE, {
				start: 42,
				end: 42,
				side: "deletions",
			}),
		).toBe("ignore");
	});

	it("cross-side selection starting on additions ending on deletions", () => {
		// User drags from the right (additions) column to the left
		// (deletions) column in split view.
		const result = commentBoxFromRange(FILE, {
			start: 10,
			end: 12,
			side: "additions",
			endSide: "deletions",
		});
		expect(result).toEqual({
			fileName: FILE,
			start: 10,
			startSide: "additions",
			end: 12,
			endSide: "deletions",
		});
	});

	it("very large line numbers are preserved", () => {
		// Big generated file: deletion around line 12000, addition
		// around line 9500.
		const result = commentBoxFromRange(FILE, {
			start: 12345,
			end: 9500,
			side: "deletions",
			endSide: "additions",
		});
		expect(result).toMatchObject({
			start: 12345,
			end: 9500,
		});
	});

	it("endSide matching side is treated as same-side", () => {
		// Library may explicitly send endSide equal to side.
		const result = commentBoxFromRange(FILE, {
			start: 5,
			end: 5,
			side: "additions",
			endSide: "additions",
		});
		expect(result).toBe("ignore");
	});

	it("backward same-side selection (start > end) is accepted", () => {
		// User shift-clicks above the anchor line.
		const result = commentBoxFromRange(FILE, {
			start: 100,
			end: 90,
			side: "deletions",
		});
		expect(result).toMatchObject({
			start: 100,
			startSide: "deletions",
			end: 90,
			endSide: "deletions",
		});
	});

	it("adjacent lines (start and end differ by 1)", () => {
		const result = commentBoxFromRange(FILE, {
			start: 7,
			end: 8,
			side: "additions",
		});
		expect(result).toMatchObject({ start: 7, end: 8 });
	});

	// -- annotationSideForBox / annotationLineForBox ------------------

	it("annotation is placed at end for backward same-side selection", () => {
		// User selects line 50 then shift-clicks line 40.
		const box: CommentBoxState = {
			fileName: FILE,
			start: 50,
			startSide: "additions",
			end: 40,
			endSide: "additions",
		};
		expect(annotationLineForBox(box)).toBe(40);
		expect(annotationSideForBox(box)).toBe("additions");
	});

	it("annotation lands on additions side for del->add cross-side", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 300,
			startSide: "deletions",
			end: 150,
			endSide: "additions",
		};
		expect(annotationSideForBox(box)).toBe("additions");
		expect(annotationLineForBox(box)).toBe(150);
	});

	it("annotation lands on deletions side for add->del cross-side", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 10,
			startSide: "additions",
			end: 15,
			endSide: "deletions",
		};
		expect(annotationSideForBox(box)).toBe("deletions");
		expect(annotationLineForBox(box)).toBe(15);
	});

	// -- selectedLinesForBox ------------------------------------------

	it("backward same-side selection preserves raw order in range", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 50,
			startSide: "deletions",
			end: 40,
			endSide: "deletions",
		};
		const range = selectedLinesForBox(box);
		expect(range.start).toBe(50);
		expect(range.end).toBe(40);
		expect(range).not.toHaveProperty("endSide");
	});

	it("cross-side with wildly different line numbers includes endSide", () => {
		// The image example: del 509, add 219.
		const box: CommentBoxState = {
			fileName: FILE,
			start: 509,
			startSide: "deletions",
			end: 219,
			endSide: "additions",
		};
		const range = selectedLinesForBox(box);
		expect(range).toEqual({
			start: 509,
			end: 219,
			side: "deletions",
			endSide: "additions",
		});
	});

	// -- contentRangeForBox -------------------------------------------

	it("same-side forward selection keeps startLine <= endLine", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 5,
			startSide: "additions",
			end: 10,
			endSide: "additions",
		};
		const { startLine, endLine } = contentRangeForBox(box);
		expect(startLine).toBe(5);
		expect(endLine).toBe(10);
	});

	it("same-side backward selection normalizes to startLine <= endLine", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 10,
			startSide: "deletions",
			end: 5,
			endSide: "deletions",
		};
		const { startLine, endLine, side } = contentRangeForBox(box);
		expect(startLine).toBe(5);
		expect(endLine).toBe(10);
		expect(side).toBe("deletions");
	});

	it("same-side single-line content range (line number click path)", () => {
		// This is the state created by handleLineNumberClick.
		const box: CommentBoxState = {
			fileName: FILE,
			start: 42,
			startSide: "additions",
			end: 42,
			endSide: "additions",
		};
		expect(contentRangeForBox(box)).toEqual({
			startLine: 42,
			endLine: 42,
			side: "additions",
		});
	});

	it("cross-side content range never produces startLine > endLine", () => {
		// Even when the deletion line number is much larger than the
		// addition line number, the content range should be a sane
		// single-line range on one side.
		const box: CommentBoxState = {
			fileName: FILE,
			start: 12345,
			startSide: "deletions",
			end: 100,
			endSide: "additions",
		};
		const { startLine, endLine, side } = contentRangeForBox(box);
		expect(startLine).toBeLessThanOrEqual(endLine);
		expect(startLine).toBe(100);
		expect(endLine).toBe(100);
		expect(side).toBe("additions");
	});

	it("cross-side add->del content range uses deletions end point", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			start: 50,
			startSide: "additions",
			end: 200,
			endSide: "deletions",
		};
		expect(contentRangeForBox(box)).toEqual({
			startLine: 200,
			endLine: 200,
			side: "deletions",
		});
	});

	// -- Round-trip: commentBoxFromRange -> helpers --------------------

	it("round-trip: cross-side range flows correctly through all helpers", () => {
		// Simulate the full lifecycle for a cross-side selection with
		// mismatched line numbers (the original bug scenario).
		const box = commentBoxFromRange(FILE, {
			start: 509,
			end: 219,
			side: "deletions",
			endSide: "additions",
		});
		if (box === null || box === "ignore") {
			throw new Error("Expected a CommentBoxState");
		}

		// Annotation at end point on end side.
		expect(annotationSideForBox(box)).toBe("additions");
		expect(annotationLineForBox(box)).toBe(219);

		// Library highlight range preserves raw coordinates.
		expect(selectedLinesForBox(box)).toEqual({
			start: 509,
			end: 219,
			side: "deletions",
			endSide: "additions",
		});

		// Content extraction uses end point only.
		const content = contentRangeForBox(box);
		expect(content.startLine).toBe(219);
		expect(content.endLine).toBe(219);
		expect(content.side).toBe("additions");
	});

	it("round-trip: same-side backward selection flows correctly", () => {
		const box = commentBoxFromRange(FILE, {
			start: 30,
			end: 20,
			side: "deletions",
		});
		if (box === null || box === "ignore") {
			throw new Error("Expected a CommentBoxState");
		}

		// Annotation at end (line 20).
		expect(annotationLineForBox(box)).toBe(20);
		expect(annotationSideForBox(box)).toBe("deletions");

		// Library range preserves raw direction.
		const range = selectedLinesForBox(box);
		expect(range.start).toBe(30);
		expect(range.end).toBe(20);
		expect(range).not.toHaveProperty("endSide");

		// Content range normalizes direction.
		expect(contentRangeForBox(box)).toEqual({
			startLine: 20,
			endLine: 30,
			side: "deletions",
		});
	});
});
