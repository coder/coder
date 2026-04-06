import { describe, expect, it } from "vitest";
import { countDiffStatLines } from "./localDiffStats";

describe("countDiffStatLines", () => {
	it("counts additions and deletions correctly", () => {
		const diff = [
			"--- a/file.txt",
			"+++ b/file.txt",
			"@@ -1,3 +1,4 @@",
			" unchanged",
			"-removed line",
			"+added line 1",
			"+added line 2",
		].join("\n");
		const stats = countDiffStatLines(diff);
		expect(stats.additions).toBe(2);
		expect(stats.deletions).toBe(1);
	});

	it("returns zeros for empty diff", () => {
		const stats = countDiffStatLines("");
		expect(stats.additions).toBe(0);
		expect(stats.deletions).toBe(0);
	});

	it("handles diff without trailing newline", () => {
		const diff = "+added line";
		const stats = countDiffStatLines(diff);
		expect(stats.additions).toBe(1);
		expect(stats.deletions).toBe(0);
	});

	it("matches split-based counting", () => {
		const diff = [
			"--- a/file.txt",
			"+++ b/file.txt",
			"@@ -1,5 +1,5 @@",
			" context",
			"-old1",
			"-old2",
			"+new1",
			"+new2",
			"+new3",
			" context",
		].join("\n");

		// Reference implementation (split-based)
		let refAdd = 0;
		let refDel = 0;
		for (const line of diff.split("\n")) {
			if (line.startsWith("+") && !line.startsWith("+++")) refAdd++;
			else if (line.startsWith("-") && !line.startsWith("---")) refDel++;
		}

		const stats = countDiffStatLines(diff);
		expect(stats.additions).toBe(refAdd);
		expect(stats.deletions).toBe(refDel);
	});
});
