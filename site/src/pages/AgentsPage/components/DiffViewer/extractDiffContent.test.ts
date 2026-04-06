import { parsePatchFiles } from "@pierre/diffs";
import { describe, expect, it } from "vitest";
import { extractDiffContent } from "../DiffViewer/CommentableDiffViewer";

function parse(diffStr: string) {
	return parsePatchFiles(diffStr).flatMap((p) => p.files);
}

/** Filter blank strings that arise from trailing newlines in parsed lines. */
function contentLines(text: string): string[] {
	return text.split("\n").filter((l) => l.length > 0);
}

// Simple diff: one context line, one changed line, one context line.
const simpleDiff = [
	"diff --git a/app.ts b/app.ts",
	"index 1111111..2222222 100644",
	"--- a/app.ts",
	"+++ b/app.ts",
	"@@ -1,3 +1,3 @@",
	" const x = 1;",
	"-const y = 2;",
	"+const y = 42;",
	" const z = 3;",
].join("\n");

// Addition-only diff: no deletions.
const additionOnlyDiff = [
	"diff --git a/add.ts b/add.ts",
	"index 1111111..2222222 100644",
	"--- a/add.ts",
	"+++ b/add.ts",
	"@@ -1,2 +1,4 @@",
	" first;",
	"+added1;",
	"+added2;",
	" last;",
].join("\n");

// Deletion-only diff: no additions.
const deletionOnlyDiff = [
	"diff --git a/del.ts b/del.ts",
	"index 1111111..2222222 100644",
	"--- a/del.ts",
	"+++ b/del.ts",
	"@@ -1,4 +1,2 @@",
	" first;",
	"-removed1;",
	"-removed2;",
	" last;",
].join("\n");

// Multi-hunk diff.
const multiHunkDiff = [
	"diff --git a/multi.ts b/multi.ts",
	"index 1111111..2222222 100644",
	"--- a/multi.ts",
	"+++ b/multi.ts",
	"@@ -1,3 +1,3 @@",
	" aaa;",
	"-bbb;",
	"+BBB;",
	" ccc;",
	"@@ -10,3 +10,3 @@",
	" xxx;",
	"-yyy;",
	"+YYY;",
	" zzz;",
].join("\n");

describe("extractDiffContent", () => {
	it("extracts addition lines from a simple change block", () => {
		const files = parse(simpleDiff);
		const result = extractDiffContent(files, "app.ts", 2, 2, "additions");
		expect(result).toContain("const y = 42;");
		expect(result).not.toContain("const y = 2;");
	});

	it("extracts deletion lines from a simple change block", () => {
		const files = parse(simpleDiff);
		const result = extractDiffContent(files, "app.ts", 2, 2, "deletions");
		expect(result).toContain("const y = 2;");
		expect(result).not.toContain("const y = 42;");
	});

	it("extracts a single line when startLine === endLine", () => {
		const files = parse(simpleDiff);
		// Line 1 is a context line on both sides.
		const result = extractDiffContent(files, "app.ts", 1, 1, "additions");
		expect(result).toContain("const x = 1;");
		expect(contentLines(result)).toHaveLength(1);
	});

	it("extracts lines spanning context and change blocks", () => {
		const files = parse(simpleDiff);
		// Lines 1-3 on the addition side: context, addition, context.
		const result = extractDiffContent(files, "app.ts", 1, 3, "additions");
		const lines = contentLines(result);
		expect(lines).toHaveLength(3);
		expect(lines[0]).toContain("const x = 1;");
		expect(lines[1]).toContain("const y = 42;");
		expect(lines[2]).toContain("const z = 3;");
	});

	it("returns empty string when range does not match any lines", () => {
		const files = parse(simpleDiff);
		const result = extractDiffContent(files, "app.ts", 100, 200, "additions");
		expect(result).toBe("");
	});

	it("returns empty string for a non-existent file name", () => {
		const files = parse(simpleDiff);
		const result = extractDiffContent(files, "nope.ts", 1, 10, "additions");
		expect(result).toBe("");
	});

	it("extracts additions from an addition-only hunk", () => {
		const files = parse(additionOnlyDiff);
		const result = extractDiffContent(files, "add.ts", 2, 3, "additions");
		const lines = contentLines(result);
		expect(lines).toHaveLength(2);
		expect(lines[0]).toContain("added1;");
		expect(lines[1]).toContain("added2;");
	});

	it("returns empty for deletions side on an addition-only hunk", () => {
		const files = parse(additionOnlyDiff);
		// The deletion side has no changed lines, so the added content
		// should never appear when asking for deletions.
		const result = extractDiffContent(files, "add.ts", 2, 3, "deletions");
		expect(result).not.toContain("added1");
		expect(result).not.toContain("added2");
	});

	it("extracts deletions from a deletion-only hunk", () => {
		const files = parse(deletionOnlyDiff);
		const result = extractDiffContent(files, "del.ts", 2, 3, "deletions");
		const lines = contentLines(result);
		expect(lines).toHaveLength(2);
		expect(lines[0]).toContain("removed1;");
		expect(lines[1]).toContain("removed2;");
	});

	it("returns empty for additions side on a deletion-only hunk", () => {
		const files = parse(deletionOnlyDiff);
		// The addition side has no changed lines, so deleted content
		// should never appear when asking for additions.
		const result = extractDiffContent(files, "del.ts", 2, 3, "additions");
		expect(result).not.toContain("removed1");
		expect(result).not.toContain("removed2");
	});

	it("extracts from the second hunk of a multi-hunk file", () => {
		const files = parse(multiHunkDiff);
		// Second hunk: addition side starts at line 10.
		const result = extractDiffContent(files, "multi.ts", 11, 11, "additions");
		expect(result).toContain("YYY;");
		expect(contentLines(result)).toHaveLength(1);
	});

	it("includes context lines when they fall in the requested range", () => {
		const files = parse(multiHunkDiff);
		// Second hunk on the addition side: lines 10 (context "xxx;"),
		// 11 (addition "YYY;"), 12 (context "zzz;").
		const result = extractDiffContent(files, "multi.ts", 10, 12, "additions");
		const lines = contentLines(result);
		expect(lines).toHaveLength(3);
		expect(lines[0]).toContain("xxx;");
		expect(lines[1]).toContain("YYY;");
		expect(lines[2]).toContain("zzz;");
	});
});
