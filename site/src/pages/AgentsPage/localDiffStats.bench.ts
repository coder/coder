import { bench, describe } from "vitest";
import { countDiffStatLines } from "./localDiffStats";

function generateDiff(lineCount: number): string {
	const lines = [
		"diff --git a/file.ts b/file.ts",
		"--- a/file.ts",
		"+++ b/file.ts",
		"@@ -1,1 +1,1 @@",
	];

	for (let i = 0; i < lineCount; i++) {
		if (i % 2 === 0) {
			lines.push(`+added line ${i}`);
		} else {
			lines.push(`-removed line ${i}`);
		}
	}

	return lines.join("\n");
}

function countDiffStatLinesSplitBased(unifiedDiff: string): {
	additions: number;
	deletions: number;
} {
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
}

describe("countDiffStatLines", () => {
	for (const lineCount of [500, 5000, 50000]) {
		const diff = generateDiff(lineCount);

		bench(`single-pass scanner (${lineCount} lines)`, () => {
			countDiffStatLines(diff);
		});

		bench(`split-based scanner (${lineCount} lines)`, () => {
			countDiffStatLinesSplitBased(diff);
		});
	}
});
