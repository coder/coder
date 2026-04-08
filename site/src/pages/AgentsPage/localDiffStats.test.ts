import { countDiffStatLines } from "./localDiffStats";

const countDiffStatLinesSplitBased = (unifiedDiff: string) => {
	let additions = 0;
	let deletions = 0;
	for (const line of unifiedDiff.split("\n")) {
		if (line.startsWith("+") && !line.startsWith("+++")) {
			additions++;
		} else if (line.startsWith("-") && !line.startsWith("---")) {
			deletions++;
		}
	}
	return { additions, deletions };
};

describe("countDiffStatLines", () => {
	it("counts additions and deletions correctly", () => {
		const diff = [
			"diff --git a/file.ts b/file.ts",
			"index 1111111..2222222 100644",
			"--- a/file.ts",
			"+++ b/file.ts",
			"@@ -1,3 +1,4 @@",
			" context line",
			"-old line",
			"+new line",
			"+another line",
		].join("\n");

		expect(countDiffStatLines(diff)).toEqual({
			additions: 2,
			deletions: 1,
		});
	});

	it("returns zeros for empty diff", () => {
		expect(countDiffStatLines("")).toEqual({ additions: 0, deletions: 0 });
	});

	it("handles diff without trailing newline", () => {
		const diff = [
			"diff --git a/file.ts b/file.ts",
			"--- a/file.ts",
			"+++ b/file.ts",
			"@@ -1 +1 @@",
			"-before",
			"+after",
		].join("\n");

		expect(countDiffStatLines(diff)).toEqual({
			additions: 1,
			deletions: 1,
		});
	});

	it("matches split-based counting", () => {
		const diff = [
			"diff --git a/a.ts b/a.ts",
			"--- a/a.ts",
			"+++ b/a.ts",
			"@@ -1,3 +1,3 @@",
			"-old one",
			"+new one",
			" unchanged",
			"@@ -10,4 +10,5 @@",
			" line",
			"-drop this",
			"+add this",
			"+add that",
			" line",
			"diff --git a/b.ts b/b.ts",
			"--- a/b.ts",
			"+++ b/b.ts",
			"@@ -2,2 +2,2 @@",
			"-alpha",
			"+beta",
		].join("\n");

		expect(countDiffStatLines(diff)).toEqual(
			countDiffStatLinesSplitBased(diff),
		);
	});
});
