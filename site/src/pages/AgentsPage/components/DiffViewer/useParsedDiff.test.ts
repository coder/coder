import { parsePatchFiles } from "@pierre/diffs";
import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { dedupeFilesByName, useParsedDiff } from "./useParsedDiff";

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

describe("useParsedDiff", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("derives a content-specific cache key when no prefix is supplied", () => {
		const { result } = renderHook(() => useParsedDiff(uniqueFilesDiff));

		expect(result.current).toHaveLength(2);
		// Keys are content-derived instead of bare file names, so a
		// later diff body for the same path cannot reuse this AST.
		for (const file of result.current) {
			expect(file.cacheKey).toMatch(/^content-/);
		}
		expect(result.current[0].cacheKey).not.toBe(result.current[1].cacheKey);
	});

	it("keeps an explicit prefix when one is supplied", () => {
		const { result } = renderHook(() =>
			useParsedDiff(uniqueFilesDiff, "chat-1-123"),
		);

		expect(result.current[0].cacheKey).toMatch(/^chat-1-123-/);
	});

	it("gives different bodies different keys", () => {
		const first = renderHook(() => useParsedDiff(uniqueFilesDiff));
		const second = renderHook(() =>
			useParsedDiff(uniqueFilesDiff.replace("const a = 2", "const a = 3")),
		);

		expect(first.result.current[0].cacheKey).not.toBe(
			second.result.current[0].cacheKey,
		);
	});
});
