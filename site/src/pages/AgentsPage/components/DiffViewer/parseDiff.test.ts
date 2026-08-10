import { parsePatchFiles } from "@pierre/diffs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { dedupeFilesByName, parseDiffString } from "./parseDiff";

// Two `diff --git` sections for the same post-image path. `parsePatchFiles`
// emits one FileDiffMetadata per section, so this is the exact shape that made
// CodeView.addItem throw `duplicate id "agent/x/agentmcp/api_internal_test.go"`
// in production.
const duplicateFileDiff = [
	"diff --git a/agent/x/agentmcp/api_internal_test.go b/agent/x/agentmcp/api_internal_test.go",
	"index 1111111..2222222 100644",
	"--- a/agent/x/agentmcp/api_internal_test.go",
	"+++ b/agent/x/agentmcp/api_internal_test.go",
	"@@ -1,3 +1,3 @@",
	" package agentmcp",
	"-const a = 1",
	"+const a = 2",
	" const b = 3",
	"diff --git a/agent/x/agentmcp/api_internal_test.go b/agent/x/agentmcp/api_internal_test.go",
	"index 3333333..4444444 100644",
	"--- a/agent/x/agentmcp/api_internal_test.go",
	"+++ b/agent/x/agentmcp/api_internal_test.go",
	"@@ -10,3 +10,3 @@",
	" const c = 4",
	"-const d = 5",
	"+const d = 6",
	" const e = 7",
].join("\n");

const uniqueFilesDiff = [
	"diff --git a/first.ts b/first.ts",
	"index 1111111..2222222 100644",
	"--- a/first.ts",
	"+++ b/first.ts",
	"@@ -1,1 +1,1 @@",
	"-const a = 1",
	"+const a = 2",
	"diff --git a/second.ts b/second.ts",
	"index 3333333..4444444 100644",
	"--- a/second.ts",
	"+++ b/second.ts",
	"@@ -1,1 +1,1 @@",
	"-const b = 1",
	"+const b = 2",
].join("\n");

function parse(diffStr: string) {
	return parsePatchFiles(diffStr).flatMap((p) => p.files);
}

describe("dedupeFilesByName", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("collapses repeated post-image paths to the first occurrence", () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
		const files = parse(duplicateFileDiff);
		// Sanity check: the parser really does hand us the duplicate that
		// crashes CodeView, so the dedupe below is exercising a real case.
		expect(files).toHaveLength(2);

		const deduped = dedupeFilesByName(files);

		expect(deduped).toHaveLength(1);
		expect(deduped[0]).toBe(files[0]);
		// Mapping to CodeView item ids (id: file.name) now yields no collision.
		expect(deduped.map((f) => f.name)).toEqual([
			"agent/x/agentmcp/api_internal_test.go",
		]);
		expect(warn).toHaveBeenCalledTimes(1);
	});

	it("preserves order and every file when paths are unique", () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
		const files = parse(uniqueFilesDiff);

		const deduped = dedupeFilesByName(files);

		expect(deduped).toEqual(files);
		expect(deduped.map((f) => f.name)).toEqual(["first.ts", "second.ts"]);
		expect(warn).not.toHaveBeenCalled();
	});

	it("returns an empty array unchanged", () => {
		expect(dedupeFilesByName([])).toEqual([]);
	});
});

describe("parseDiffString", () => {
	it("returns an empty array for empty input", () => {
		expect(parseDiffString(null)).toEqual([]);
		expect(parseDiffString(undefined)).toEqual([]);
		expect(parseDiffString("")).toEqual([]);
	});

	it("keys each file by its own parsed content", () => {
		const files = parseDiffString(uniqueFilesDiff);

		expect(files).toHaveLength(2);
		// A later diff body for the same path must not reuse this AST.
		for (const file of files) {
			expect(file.cacheKey).toMatch(/^content-[0-9a-f]+-[0-9a-f]+$/);
		}
		expect(files[0].cacheKey).not.toBe(files[1].cacheKey);
	});

	it("keeps an unchanged file's key stable when a sibling changes", () => {
		const changed = uniqueFilesDiff.replace("const b = 2", "const b = 3");

		const before = parseDiffString(uniqueFilesDiff);
		const after = parseDiffString(changed);

		expect(before[0].cacheKey).toBe(after[0].cacheKey);
		expect(before[1].cacheKey).not.toBe(after[1].cacheKey);
	});

	it("gives the same path different keys for different bodies", () => {
		const first = parseDiffString(duplicateFileDiff);
		// Dedupe keeps the first section; parse the second body alone to
		// compare keys for one path with two bodies.
		const secondBody = duplicateFileDiff
			.split("diff --git")
			.slice(2)
			.map((s) => `diff --git${s}`)
			.join("");
		const second = parseDiffString(secondBody);

		expect(first[0].name).toBe(second[0].name);
		expect(first[0].cacheKey).not.toBe(second[0].cacheKey);
	});
});
