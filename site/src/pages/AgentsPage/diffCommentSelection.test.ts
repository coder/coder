import { describe, expect, it } from "vitest";
import {
	type CommentBoxState,
	annotationSideForBox,
	commentBoxFromRange,
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
		// This is the core regression case: in split diff view, selecting
		// from additions line 16 to deletions line 16 spans two visual
		// rows but has the same numeric line number on each side.
		const result = commentBoxFromRange(FILE, {
			start: 16,
			end: 16,
			side: "additions",
			endSide: "deletions",
		});
		expect(result).toEqual({
			fileName: FILE,
			startLine: 16,
			endLine: 16,
			side: "additions",
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
			startLine: 10,
			endLine: 14,
			side: "deletions",
		});
	});

	it("creates a comment box for a multi-line cross-side selection", () => {
		const result = commentBoxFromRange(FILE, {
			start: 10,
			end: 14,
			side: "deletions",
			endSide: "additions",
		});
		expect(result).toEqual({
			fileName: FILE,
			startLine: 10,
			endLine: 14,
			side: "deletions",
			endSide: "additions",
		});
	});

	it("normalizes start/end so startLine <= endLine", () => {
		const result = commentBoxFromRange(FILE, {
			start: 20,
			end: 15,
			side: "additions",
		});
		expect(result).toMatchObject({ startLine: 15, endLine: 20 });
	});

	it("defaults side to additions when omitted", () => {
		const result = commentBoxFromRange(FILE, { start: 3, end: 7 });
		expect(result).toMatchObject({ side: "additions" });
	});
});

describe("annotationSideForBox", () => {
	it("returns the start side when there is no endSide", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			startLine: 1,
			endLine: 5,
			side: "deletions",
		};
		expect(annotationSideForBox(box)).toBe("deletions");
	});

	it("returns endSide for cross-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			startLine: 16,
			endLine: 16,
			side: "additions",
			endSide: "deletions",
		};
		expect(annotationSideForBox(box)).toBe("deletions");
	});
});

describe("selectedLinesForBox", () => {
	it("builds a range without endSide for same-side selections", () => {
		const box: CommentBoxState = {
			fileName: FILE,
			startLine: 3,
			endLine: 8,
			side: "additions",
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
			startLine: 16,
			endLine: 16,
			side: "additions",
			endSide: "deletions",
		};
		expect(selectedLinesForBox(box)).toEqual({
			start: 16,
			end: 16,
			side: "additions",
			endSide: "deletions",
		});
	});
});
